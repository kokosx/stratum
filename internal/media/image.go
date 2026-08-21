package media

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"

	"golang.org/x/image/draw"
	// Register WebP decoding so uploaded WebP sources can be read. We still
	// re-encode variants losslessly (PNG) because stdlib has no WebP encoder.
	_ "golang.org/x/image/webp"
)

// maxImageBytes is the hard limit on an uploaded image's decoded byte size.
const maxImageBytes = 10 << 20

// targetWidths are the responsive sizes generated for raster images. Variants
// larger than the original are skipped (never upscaled) and only a handful are
// produced so we don't create dozens of thumbnails.
var targetWidths = []int{480, 768, 1280, 1920}

// faviconSizes are the square sizes derived from the Site Icon asset.
var faviconSizes = []int{16, 32, 180, 192, 512}

const (
	jpegQuality = 82
	// variantMime is the fallback format used for generated derivatives when the
	// source format cannot be re-encoded losslessly with the standard library.
	// The system stays pure-Go and single-binary; a WebP encoder can be wired in
	// later behind encodeVariant without changing callers.
	variantMime = "image/png"
)

// allowedFormats is the upload whitelist. SVG is intentionally excluded from the
// first slice; it would need sanitization before it can be trusted.
var allowedFormats = map[string]bool{
	"jpeg": true,
	"png":  true,
	"gif":  true,
	"webp": true,
}

// Processed is the result of analyzing and deriving variants from an upload.
type Processed struct {
	Original  []byte
	Format    string
	Width     int
	Height    int
	Variants  []VariantBytes
}

// VariantBytes holds one generated derivative before it is persisted.
type VariantBytes struct {
	Width  int
	Height int
	Mime   string
	Data   []byte
}

// ProcessImage verifies the real format of data (never trusting the extension),
// and produces resized derivatives. GIFs are returned unchanged (animation is
// preserved); raster images get responsive variants in the source-encodable
// format.
func ProcessImage(data []byte) (*Processed, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("media: empty upload")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("media: cannot read image: %w", err)
	}
	if !allowedFormats[format] {
		return nil, ErrUnsupportedFormat
	}

	// GIFs keep their original bytes; resizing would drop the animation.
	if format == "gif" {
		return &Processed{Original: data, Format: format, Width: cfg.Width, Height: cfg.Height}, nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("media: cannot decode image: %w", err)
	}

	variants := make([]VariantBytes, 0, len(targetWidths))
	for _, w := range targetWidths {
		if w >= cfg.Width {
			continue
		}
		h := cfg.Height * w / cfg.Width
		if h < 1 {
			h = 1
		}
		variant, err := encodeVariant(img, w, h, variantMimeFor(format))
		if err != nil {
			return nil, err
		}
		variants = append(variants, variant)
	}
	return &Processed{Original: data, Format: format, Width: cfg.Width, Height: cfg.Height, Variants: variants}, nil
}

// variantMimeFor picks a lossless-enough, stdlib-encodable variant format. JPEG
// sources stay JPEG; PNG and WebP sources are re-encoded as PNG (so transparency
// survives). GIFs are never resized.
func variantMimeFor(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "png", "webp":
		return "image/png"
	default:
		return variantMime
	}
}

// FaviconVariant derives a square, resized PNG of the given size from an image.
// Non-square sources are center-cropped to a square. It returns the bytes and
// mime type to store.
func FaviconVariant(data []byte, size int) ([]byte, string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("media: cannot decode image: %w", err)
	}
	square := cropSquare(img)
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), square, square.Bounds(), draw.Src, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, "", fmt.Errorf("media: encode favicon: %w", err)
	}
	return buf.Bytes(), "image/png", nil
}

// cropSquare returns the largest centered square region of m.
func cropSquare(m image.Image) image.Image {
	b := m.Bounds()
	min := b.Min
	w := b.Dx()
	h := b.Dy()
	side := w
	if h < side {
		side = h
	}
	x0 := min.X + (w-side)/2
	y0 := min.Y + (h-side)/2
	return m.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(image.Rect(x0, y0, x0+side, y0+side))
}

// encodeVariant resizes src to w×h and encodes it. The encoder is intentionally a
// single, swappable function so a WebP backend can replace the stdlib path later.
func encodeVariant(src image.Image, w, h int, mime string) (VariantBytes, error) {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)

	var buf bytes.Buffer
	switch mime {
	case "image/png":
		if err := png.Encode(&buf, dst); err != nil {
			return VariantBytes{}, fmt.Errorf("media: encode png: %w", err)
		}
	case "image/jpeg":
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return VariantBytes{}, fmt.Errorf("media: encode jpeg: %w", err)
		}
	default:
		return VariantBytes{}, fmt.Errorf("media: unsupported variant mime %q", mime)
	}
	return VariantBytes{Width: w, Height: h, Mime: mime, Data: buf.Bytes()}, nil
}

// readLimited reads up to limit+1 bytes so the caller can detect oversize uploads.
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit+1))
}
