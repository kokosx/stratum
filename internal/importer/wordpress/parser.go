package wordpress

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxItemContentBytes = 16 << 20

// parse streams a WXR document. It never unmarshals the enclosing export.
func parse(path string, onItem func(item) error, onTerm func(term) error, onAuthor func(author) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	d := xml.NewDecoder(f)
	d.Strict = true
	// Prevent entity expansion attacks: do not expand external entities
	d.Entity = xml.HTMLEntity
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
			var raw wxrItem
			if err := d.DecodeElement(&raw, &start); err != nil {
				return err
			}
			contentLen := len(raw.Content) + len(raw.ContentAlt) + len(raw.ExcerptContent) + len(raw.ExcerptContentAlt) + len(raw.Excerpt)
			if contentLen > maxItemContentBytes {
				return fmt.Errorf("WXR item %s content exceeds %d bytes", raw.ID, maxItemContentBytes)
			}
			// Prefer content:encoded, fallback to excerpt if content empty? Actually keep both.
			// For WXR, Content holds content:encoded, ExcerptContent holds excerpt:encoded
			if raw.Excerpt != "" && raw.ExcerptContent == "" {
				raw.ExcerptContent = raw.Excerpt
			}
			if onItem != nil {
				it := raw.item()
				if err := onItem(it); err != nil {
					return err
				}
			}
		case "category", "tag":
			var raw wxrTerm
			if err := d.DecodeElement(&raw, &start); err != nil {
				return err
			}
			if raw.ID != "" && onTerm != nil {
				if err := onTerm(raw.term(start.Name.Local)); err != nil {
					return err
				}
			}
		case "author":
			var raw wxrAuthor
			if err := d.DecodeElement(&raw, &start); err != nil {
				return err
			}
			if raw.Login != "" && onAuthor != nil {
				if err := onAuthor(author{Login: raw.Login, Email: raw.Email}); err != nil {
					return err
				}
			}
		}
	}
}

type wxrItem struct {
	Title             string       `xml:"title"`
	Content           string       `xml:"http://purl.org/rss/1.0/modules/content encoded"`
	ContentAlt        string       `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	Excerpt           string       `xml:"excerpt"` // fallback plain excerpt if present
	ExcerptContent    string       `xml:"http://wordpress.org/export/1.2/excerpt encoded"`
	ExcerptContentAlt string       `xml:"http://wordpress.org/export/1.2/excerpt/ encoded"`
	ID                string       `xml:"post_id"`
	Type              string       `xml:"post_type"`
	Status            string       `xml:"status"`
	Slug              string       `xml:"post_name"`
	ParentID          string       `xml:"post_parent"`
	Author            string       `xml:"creator"`
	Password          string       `xml:"post_password"`
	Date              string       `xml:"post_date_gmt"`
	Modified          string       `xml:"post_modified_gmt"`
	MenuOrder         string       `xml:"menu_order"`
	AttachmentURL     string       `xml:"attachment_url"`
	Categories        []wxrTermRef `xml:"category"`
	Meta              []wxrMeta    `xml:"postmeta"`
	Comments          []struct{}   `xml:"comment"`
}

func (w wxrItem) item() item {
	content := w.Content
	if content == "" {
		content = w.ContentAlt
	}
	excerpt := w.ExcerptContent
	if excerpt == "" {
		excerpt = w.ExcerptContentAlt
	}
	if excerpt == "" {
		excerpt = w.Excerpt
	}
	i := item{ID: strings.TrimSpace(w.ID), Type: strings.TrimSpace(w.Type), Status: strings.TrimSpace(w.Status), Title: strings.TrimSpace(w.Title), Content: content, Excerpt: excerpt, Slug: strings.TrimSpace(w.Slug), ParentID: strings.TrimSpace(w.ParentID), Author: strings.TrimSpace(w.Author), Password: w.Password, AttachmentURL: strings.TrimSpace(w.AttachmentURL), Meta: map[string]string{}, Comments: len(w.Comments)}
	i.PublishedAt = parseDate(w.Date)
	i.ModifiedAt = parseDate(w.Modified)
	i.MenuOrder, _ = strconv.ParseInt(strings.TrimSpace(w.MenuOrder), 10, 64)
	for _, t := range w.Categories {
		i.Terms = append(i.Terms, termRef{Domain: t.Domain, Slug: t.Slug, Name: t.Name})
	}
	for _, m := range w.Meta {
		if m.Key == "_thumbnail_id" {
			i.Meta[m.Key] = strings.TrimSpace(m.Value)
		}
	}
	return i
}

type wxrTermRef struct {
	Domain string `xml:"domain,attr"`
	Slug   string `xml:"nicename,attr"`
	Name   string `xml:",chardata"`
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
