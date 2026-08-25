// Package wordpress imports WordPress WXR exports into normal Stratum records.
package wordpress

import "time"

type item struct {
	ID, Type, Status, Title, Content, Excerpt, Slug, ParentID, Author, Password string
	PublishedAt, ModifiedAt                                                     time.Time
	MenuOrder                                                                   int64
	Terms                                                                       []termRef
	Meta                                                                        map[string]string
	AttachmentURL                                                               string
	Comments                                                                    int
}

type termRef struct {
	Domain, Slug, Name string
}

type term struct {
	ID, Kind, Name, Slug, Parent, Description string
}

type author struct {
	Login, Email string
}
