package rendering

import "context"

// MediaView is the resolver output the block templates consume. The renderer
// never touches storage or the database directly; it asks the MediaProvider to
// turn a mediaId into the URLs and dimensions needed for an <img>.
type MediaView struct {
	ID     string
	Src    string
	SrcSet string
	Width  int
	Height int
	Alt    string
}

// MediaProvider resolves a media asset id into a MediaView. It is implemented by
// the media service so the rendering layer depends only on this small interface.
type MediaProvider interface {
	MediaView(ctx context.Context, id string) (MediaView, bool)
}

// FaviconView carries the generated site-icon URLs the theme emits as <link>s.
type FaviconView struct {
	Size16  string
	Size32  string
	Size180 string
	Size192 string
	Size512 string
}
