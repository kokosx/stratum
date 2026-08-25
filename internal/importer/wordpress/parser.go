package wordpress

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// maxItemContentBytes bounds ALL textual payload accumulated for one <item>.
// Enforcement happens during streaming accumulation, so an oversized item fails
// WITHOUT ever materializing its full body in memory.
const maxItemContentBytes = 16 << 20

// XML namespaces used by WXR (trailing-slash tolerant on lookup).
const (
	contentNSPrefix = "http://purl.org/rss/1.0/modules/content"
	excerptNSPrefix = "http://wordpress.org/export/1.2/excerpt"
	wpNSSuffix      = "http://wordpress.org/export/1.2"
	dcNSPrefix      = "http://purl.org/dc/elements/1.1"
)

func trimNS(space string) string { return strings.TrimSuffix(space, "/") }

// type aliases keep the streaming parser decoupled from the domain model.
type wxrTermRef = termRef

type wxrComment struct {
	ID       string
	Author   string
	Email    string
	URL      string
	Content  string
	Date     string
	Approved string
	Parent   string
	Type     string
}

var errItemTooLarge = errors.New("WXR item exceeds maximum supported size")

// parse streams a WXR document. It never unmarshals the enclosing export.
// encoding/xml rejects undefined entities by default, so malicious
// DOCTYPE/user-entity input fails instead of expanding (no XXE surface).
func parse(path string, onItem func(item) error, onTerm func(term) error, onAuthor func(author) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	d := xml.NewDecoder(f)
	d.Strict = true
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("decode WXR: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "item":
			raw, perr := decodeBoundedItem(d)
			if perr != nil {
				return perr
			}
			if onItem != nil {
				if err := onItem(raw.item()); err != nil {
					return err
				}
			}
		case "category", "tag":
			var t wxrTerm
			if err := d.DecodeElement(&t, &start); err != nil {
				return err
			}
			if t.ID != "" && onTerm != nil {
				if err := onTerm(t.term(start.Name.Local)); err != nil {
					return err
				}
			}
		case "author":
			var a wxrAuthor
			if err := d.DecodeElement(&a, &start); err != nil {
				return err
			}
			if a.Login != "" && onAuthor != nil {
				if err := onAuthor(author{Login: a.Login, Email: a.Email}); err != nil {
					return err
				}
			}
		}
	}
}

// nodeKey canonicalizes an element by namespace+local name to a field key.
func nodeKey(sp, local string) string {
	ns := trimNS(sp)
	switch {
	case ns == contentNSPrefix && local == "encoded":
		return "content"
	case ns == excerptNSPrefix && local == "encoded":
		return "excerpt_encoded"
	case ns == wpNSSuffix:
		return local
	case ns == dcNSPrefix && local == "creator":
		return "creator"
	default:
		return local
	}
}

type node struct {
	key    string // canonical key
	parent string // parent canonical key ("" at item root)
}

// decodeBoundedItem walks one <item> token-by-token with capped buffers, so peak
// memory stays proportional to accepted content, never to input size.
func decodeBoundedItem(d *xml.Decoder) (wxrItem, error) {
	var raw wxrItem
	var stack []node
	var buf strings.Builder
	total := 0

	write := func(p []byte) error {
		const margin = len("<wp:post_password></wp:post_password>")
		if total+len(p)+margin > maxItemContentBytes {
			return errItemTooLarge
		}
		total += len(p)
		buf.Write(p)
		return nil
	}

	for depth := 0; ; depth++ {
		tok, err := d.Token()
		if err != nil {
			return raw, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			key := nodeKey(t.Name.Space, t.Name.Local)
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1].key
			}
			buf.Reset()
			stack = append(stack, node{key: key, parent: parent})
			if t.Name.Local == "category" {
				var ref wxrTermRef
				for _, a := range t.Attr {
					switch strings.ToLower(a.Name.Local) {
					case "domain":
						ref.Domain = a.Value
					case "nicename":
						ref.Slug = a.Value
					}
				}
				raw.pendingCat = &ref
			}
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			if err := write(t); err != nil {
				return raw, err
			}
		case xml.EndElement:
			text := strings.TrimSpace(buf.String())
			buf.Reset()
			if len(stack) == 0 {
				if t.Name.Local == "item" {
					return raw, nil
				}
				continue
			}
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch {
			case cur.key == "category":
				name := ""
				if raw.pendingCat != nil {
					raw.pendingCat.Name = text
					raw.Categories = append(raw.Categories, *raw.pendingCat)
					raw.pendingCat = nil
				}
				_ = name
			case cur.parent == "postmeta":
				assignPostmeta(&raw.pendingMeta, cur.key, text)
			case cur.parent == "comment":
				assignCommentField(&raw.pendingComment, cur.key, text)
			case cur.key == "postmeta", cur.key == "comment":
				flushContainer(&raw, cur.key)
			default:
				assignItemField(&raw, cur.key, text)
			}
		}
	}
}

func assignPostmeta(m *wxrMeta, key, value string) {
	switch key {
	case "meta_key":
		m.Key = value
	case "meta_value":
		m.Value = value
	}
}

func assignCommentField(c *wxrComment, key, value string) {
	switch key {
	case "comment_id":
		c.ID = value
	case "comment_author":
		c.Author = value
	case "comment_author_email":
		c.Email = value
	case "comment_author_url":
		c.URL = value
	case "comment_content":
		c.Content = value
	case "comment_date_gmt":
		c.Date = value
	case "comment_approved":
		c.Approved = value
	case "comment_parent":
		c.Parent = value
	case "comment_type":
		c.Type = value
	}
}

func flushContainer(raw *wxrItem, which string) {
	switch which {
	case "postmeta":
		raw.Meta = append(raw.Meta, raw.pendingMeta)
		raw.pendingMeta = wxrMeta{}
	case "comment":
		if raw.pendingComment.ID != "" {
			raw.Comments = append(raw.Comments, raw.pendingComment)
		}
		raw.pendingComment = wxrComment{}
	}
}

func assignItemField(raw *wxrItem, key, value string) {
	switch key {
	case "title":
		raw.Title = value
	case "content":
		raw.Content = value
	case "excerpt":
		raw.Excerpt = value
	case "excerpt_encoded":
		raw.ExcerptContent = value
	case "post_id":
		raw.ID = value
	case "post_type":
		raw.Type = value
	case "status":
		raw.Status = value
	case "post_name":
		raw.Slug = value
	case "post_parent":
		raw.ParentID = value
	case "creator":
		raw.Author = value
	case "post_password":
		raw.Password = value
	case "post_date_gmt":
		raw.Date = value
	case "post_modified_gmt":
		raw.Modified = value
	case "menu_order":
		raw.MenuOrder = value
	case "attachment_url":
		raw.AttachmentURL = value
	}
}

type wxrItem struct {
	Title          string
	Content        string
	Excerpt        string
	ExcerptContent string
	ID             string
	Type           string
	Status         string
	Slug           string
	ParentID       string
	Author         string
	Password       string
	Date           string
	Modified       string
	MenuOrder      string
	AttachmentURL  string
	Categories     []wxrTermRef
	Meta           []wxrMeta
	Comments       []wxrComment
	pendingMeta    wxrMeta
	pendingComment wxrComment
	pendingCat     *wxrTermRef
}

func (w wxrItem) item() item {
	content := w.Content
	excerpt := w.ExcerptContent
	if excerpt == "" {
		excerpt = w.Excerpt
	}
	i := item{
		ID: strings.TrimSpace(w.ID), Type: strings.TrimSpace(w.Type), Status: strings.TrimSpace(w.Status),
		Title: strings.TrimSpace(w.Title), Content: content, Excerpt: excerpt,
		Slug: strings.TrimSpace(w.Slug), ParentID: strings.TrimSpace(w.ParentID),
		Author: strings.TrimSpace(w.Author), Password: w.Password,
		AttachmentURL: strings.TrimSpace(w.AttachmentURL), Meta: map[string]string{},
	}
	i.PublishedAt = parseDate(w.Date)
	i.ModifiedAt = parseDate(w.Modified)
	i.MenuOrder, _ = strconv.ParseInt(strings.TrimSpace(w.MenuOrder), 10, 64)
	i.Terms = append(i.Terms, w.Categories...)
	for _, m := range w.Meta {
		if m.Key == "_thumbnail_id" && strings.TrimSpace(m.Value) != "" {
			i.Meta[m.Key] = strings.TrimSpace(m.Value)
		}
	}
	for _, c := range w.Comments {
		i.Comments = append(i.Comments, importComment{
			ID: strings.TrimSpace(c.ID), ParentID: strings.TrimSpace(c.Parent),
			Author: strings.TrimSpace(c.Author), Email: strings.TrimSpace(c.Email),
			URL: strings.TrimSpace(c.URL), Content: c.Content,
			CreatedAt: parseDate(c.Date), Approved: strings.TrimSpace(c.Approved),
			Type: strings.TrimSpace(c.Type),
		})
	}
	return i
}

type wxrMeta struct {
	Key   string `xml:"meta_key"`
	Value string `xml:"meta_value"`
}
type wxrTerm struct {
	ID             string `xml:"term_id"`
	Name           string `xml:"cat_name"`
	Slug           string `xml:"category_nicename"`
	Parent         string `xml:"category_parent"`
	Description    string `xml:"cat_description"`
	TagName        string `xml:"tag_name"`
	TagSlug        string `xml:"tag_slug"`
	TagDescription string `xml:"tag_description"`
}

func (w wxrTerm) term(kind string) term {
	if kind == "tag" {
		return term{ID: w.ID, Kind: "tag", Name: w.TagName, Slug: w.TagSlug, Description: w.TagDescription}
	}
	return term{ID: w.ID, Kind: "category", Name: w.Name, Slug: w.Slug, Parent: w.Parent, Description: w.Description}
}

type wxrAuthor struct {
	Login string `xml:"author_login"`
	Email string `xml:"author_email"`
}

func parseDate(v string) time.Time {
	v = strings.TrimSpace(v)
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02 15:04:05.000000"} {
		if t, err := time.ParseInLocation(layout, v, time.UTC); err == nil {
			return t
		}
	}
	return time.Time{}
}
