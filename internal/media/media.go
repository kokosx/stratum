// Package media is the central asset domain for StratumCMS. Blocks, site
// settings and the renderer reference assets by id; the service stores bytes in
// controlled storage and keeps metadata in the database.
package media

import (
	"errors"
	"time"
)

// AssetType classifies an uploaded asset. The first slice is image-centric, but
// the model is intentionally not hard-wired to images.
type AssetType string

const (
	AssetTypeImage AssetType = "image"
	AssetTypeVideo AssetType = "video"
	AssetTypeAudio AssetType = "audio"
	AssetTypeDoc   AssetType = "document"
	AssetTypeOther AssetType = "other"
)

var (
	// ErrUnsupportedFormat means the upload is not in an allowed, verified format.
	ErrUnsupportedFormat = errors.New("media: unsupported or unverified file format")
	// ErrTooLarge means the upload exceeded the configured size limit.
	ErrTooLarge = errors.New("media: upload exceeds the maximum allowed size")
	// ErrInUse means the asset is referenced by content and cannot be deleted
	// without an explicit force flag.
	ErrInUse = errors.New("media: asset is still used by content")
	// ErrMalformed means the file is malformed or cannot be parsed.
	ErrMalformed = errors.New("media: malformed file")
	// ErrInvalidImage means the file contains invalid image data.
	ErrInvalidImage = errors.New("media: invalid image data")
	// ErrDimensionsTooLarge means the image dimensions exceed the allowed limit.
	ErrDimensionsTooLarge = errors.New("media: image dimensions too large")
	// ErrTooManyPixels means the image has too many pixels.
	ErrTooManyPixels = errors.New("media: image has too many pixels")
	// ErrSVGUnsafe means the SVG contains unsafe or unsupported content.
	ErrSVGUnsafe = errors.New("media: SVG upload rejected: the file contains unsupported or unsafe elements")
	// ErrDerivativeFailed means variant generation failed.
	ErrDerivativeFailed = errors.New("media: derivative generation failed")
)

// Asset is the full media record with its generated variants attached.
type Asset struct {
	ID               string
	OriginalFilename string
	StorageKey       string
	MimeType         string
	AssetType        string
	FileSize         int64
	Width            int
	Height           int
	AltText          string
	Title            string
	Caption          string
	Description      string
	AuthorID         string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Variants         []Variant
}

// Variant is one generated derivative of an asset (a responsive size, its WebP
// twin, a favicon size, a future AVIF, ...). kind is the URL slug used to serve
// it; responsive WebP twins carry a ".webp" suffix (e.g. "480.webp").
// ContentHash is a short digest of the stored bytes: resolvers append it to
// variant URLs (?v=...) so regenerated derivatives are never served stale from
// an immutable-cached URL.
type Variant struct {
	ID          string
	MediaID     string
	Kind        string
	StorageKey  string
	MimeType    string
	Width       int
	Height      int
	FileSize    int64
	ContentHash string
}

// Usage describes where an asset is referenced, for the delete-confirmation UI.
type Usage struct {
	Count int64
}
