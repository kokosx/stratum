package media

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"regexp"
	"strconv"
	"strings"

	webp "github.com/HugoSmits86/nativewebp"
	"golang.org/x/image/draw"
	// Register WebP decoding so uploaded WebP sources can be read. Derivatives
	// are re-encoded with the pure-Go nativewebp encoder (lossless VP8L), so
	// deployment stays a single static binary with no external tools.
	_ "golang.org/x/image/webp"
)

// maxImageBytes is the hard limit on an uploaded image's decoded byte size.
const maxImageBytes = 10 << 20

// maxImageDimension and maxImagePixels guard against decompression bombs: an
// image is rejected before it is fully decoded if either its width/height or
// its total pixel count is implausibly large.
const (
	maxImageDimension = 20000
	maxImagePixels    = 100_000_000
)

// targetWidths are the responsive sizes generated for raster images. Variants
// larger than the original are skipped (never upscaled) and only a handful are
// produced so we don't create dozens of thumbnails.
var targetWidths = []int{480, 768, 1280, 1920}

// faviconSizes are the square sizes derived from the Site Icon asset.
var faviconSizes = []int{16, 32, 180, 192, 512}

// socialWidth/Height define the dedicated Open Graph preview derivative. The
// pipeline keeps the original file untouched and stores the cropped, resized
// derivative as a separate media_variant with kind=socialKind. The resolver
// picks this single URL (never a srcset) as og:image.
const (
	socialWidth  = 1200
	socialHeight = 630
	socialKind   = "social"
)

// FocalPoint selects the crop focus inside the source image. Coordinates are
// normalized (0..1) where 0.5,0.5 is the center. The current pipeline uses the
// center crop, but the type is kept so a future UI can persist per-asset
// focal points without changing the variant generation contract.
type FocalPoint struct {
	X float64 // 0 = left, 0.5 = center, 1 = right
	Y float64 // 0 = top,  0.5 = center, 1 = bottom
}

const (
	jpegQuality = 82
	// webpMime is the format of the modern derivative set. Variants are
	// encoded lossless (VP8L) with a pure-Go encoder, keeping the build cgo-free.
	webpMime = "image/webp"
	// variantMime is the fallback format used for generated derivatives when the
	// source format has no dedicated responsive encoding path.
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
	Original []byte
	Format   string
	Width    int
	Height   int
	Variants []VariantBytes
}

// VariantBytes holds one generated derivative before it is persisted.
// Kind is the URL slug used to serve the variant. When empty the kind is
// derived from Width (responsive variants like "480"); the social preview uses
// the dedicated kind "social" and must not appear in srcsets.
type VariantBytes struct {
	Kind   string
	Width  int
	Height int
	Mime   string
	Data   []byte
}

// isSVG reports whether data looks like an SVG document.
func isSVG(data []byte) bool {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return false
	}
	// Strip BOM
	trimmed = strings.TrimPrefix(trimmed, "\xEF\xBB\xBF")
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "<?xml") {
		// Must contain <svg
		return strings.Contains(lower, "<svg")
	}
	return strings.HasPrefix(lower, "<svg")
}

// sanitizeSVG parses and validates SVG, rejecting unsafe content.
// It uses a denylist approach for dangerous elements/attributes and
// validates XML well-formedness. On success it returns the sanitized bytes
// (currently the original if safe, as we strip nothing but validate).
func sanitizeSVG(data []byte) ([]byte, error) {
	// Check well-formed XML
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	dangerousElements := map[string]bool{
		"script": true, "foreignobject": true, "iframe": true, "object": true, "embed": true, "link": true, "meta": true, "style": true, // style can contain @import
	}
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: malformed SVG: %v", ErrMalformed, err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(el.Name.Local)
			if dangerousElements[name] {
				return nil, fmt.Errorf("%w: disallowed element <%s>", ErrSVGUnsafe, name)
			}
			for _, attr := range el.Attr {
				attrName := strings.ToLower(attr.Name.Local)
				attrVal := strings.ToLower(attr.Value)
				// Event handlers
				if strings.HasPrefix(attrName, "on") {
					return nil, fmt.Errorf("%w: event handler attribute %q is not allowed", ErrSVGUnsafe, attr.Name.Local)
				}
				if strings.Contains(attrVal, "javascript:") {
					return nil, fmt.Errorf("%w: javascript: URL is not allowed", ErrSVGUnsafe)
				}
				if attrName == "href" || attrName == "xlink:href" || strings.HasSuffix(attrName, "href") {
					if strings.HasPrefix(strings.TrimSpace(attrVal), "data:text/html") || strings.HasPrefix(strings.TrimSpace(attrVal), "data:application") {
						return nil, fmt.Errorf("%w: unsafe data URL", ErrSVGUnsafe)
					}
					// Allow internal fragment (#) and relative, but reject external http with potential tracking? We allow http for images but flag?
					// For now, reject external http(s) to avoid remote resource loading (per spec: no arbitrary remote resource loading)
					if strings.HasPrefix(attrVal, "http://") || strings.HasPrefix(attrVal, "https://") {
						// Allow if it's an image? But spec says no arbitrary remote resource loading, so reject.
						return nil, fmt.Errorf("%w: external resource %q is not allowed", ErrSVGUnsafe, attr.Value)
					}
				}
			}
		}
	}
	// Also check for javascript: case-insensitive in raw data as fallback
	lower := strings.ToLower(string(data))
	if strings.Contains(lower, "javascript:") {
		return nil, fmt.Errorf("%w: javascript: URL is not allowed", ErrSVGUnsafe)
	}
	// If we reach here, consider safe. Return original (preserve safe vector graphics).
	return data, nil
}

var svgDimensionRe = regexp.MustCompile(`(?i)(?:width|height)\s*=\s*["']?\s*(\d+(?:\.\d+)?)`)

func parseSVGDimensions(data []byte) (int, int) {
	// Try to parse width/height from SVG attributes, fallback to viewBox
	matches := svgDimensionRe.FindAllStringSubmatch(string(data), 2)
	var w, h int
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		val, _ := strconv.ParseFloat(m[1], 64)
		if strings.Contains(strings.ToLower(m[0]), "width") && w == 0 {
			w = int(val)
		} else if strings.Contains(strings.ToLower(m[0]), "height") && h == 0 {
			h = int(val)
		}
	}
	if w > 0 && h > 0 {
		return w, h
	}
	// Try viewBox="0 0 100 100"
	re := regexp.MustCompile(`(?i)viewBox\s*=\s*["']\s*[\d\.\-]+\s+[\d\.\-]+\s+([\d\.]+)\s+([\d\.]+)\s*["']`)
	if m := re.FindStringSubmatch(string(data)); len(m) == 3 {
		vw, _ := strconv.ParseFloat(m[1], 64)
		vh, _ := strconv.ParseFloat(m[2], 64)
		return int(vw), int(vh)
	}
	return w, h
}

// ProcessImage verifies the real format of data (never trusting the extension),
// and produces resized derivatives. GIFs are returned unchanged (animation is
// preserved); raster images get responsive variants plus a WebP derivative set
// (always for PNG/WebP sources, size-gated for JPEG sources). SVGs are
// sanitized and returned as vector-only assets.
func ProcessImage(data []byte) (*Processed, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty upload", ErrMalformed)
	}
	if int64(len(data)) > maxImageBytes {
		return nil, ErrTooLarge
	}
	// SVG handling first (before image.DecodeConfig which would fail for SVG)
	if isSVG(data) {
		sanitized, err := sanitizeSVG(data)
		if err != nil {
			return nil, err
		}
		w, h := parseSVGDimensions(sanitized)
		if w > maxImageDimension || h > maxImageDimension {
			return nil, ErrDimensionsTooLarge
		}
		if int64(w)*int64(h) > maxImagePixels && w > 0 && h > 0 {
			return nil, ErrTooManyPixels
		}
		return &Processed{Original: sanitized, Format: "svg", Width: w, Height: h, Variants: nil}, nil
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read image: %v", ErrMalformed, err)
	}
	if !allowedFormats[format] {
		return nil, ErrUnsupportedFormat
	}

	// Reject absurd resolutions before the (expensive) full decode. DecodeConfig
	// only reads the header, so this is a cheap decompression-bomb guard.
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImageDimension || cfg.Height > maxImageDimension {
		return nil, ErrDimensionsTooLarge
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil, ErrTooManyPixels
	}

	// GIFs keep their original bytes; resizing would drop the animation.
	if format == "gif" {
		return &Processed{Original: data, Format: format, Width: cfg.Width, Height: cfg.Height}, nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: cannot decode image: %v", ErrInvalidImage, err)
	}
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
		nativeMime := responsiveMimeFor(format)
		if nativeMime == webpMime {
			// WebP source: the modern set is also WebP, so a second (usually
			// much larger PNG) fallback set would only waste storage.
			wv, err := encodeVariant(img, w, h, webpMime)
			if err != nil {
				return nil, err
			}
			wv.Kind = strconv.Itoa(w) + webpKindExt
			variants = append(variants, wv)
			continue
		}
		nv, err := encodeVariant(img, w, h, nativeMime)
		if err != nil {
			return nil, err
		}
		nv.Kind = strconv.Itoa(w)
		variants = append(variants, nv)
		// WebP twin for JPEG/PNG sources. Kept only when it actually saves
		// bytes: lossless WebP beats PNG almost always, but photographic
		// JPEG content usually compresses better as JPEG, and shipping a
		// bigger "modern" variant would be a regression.
		if wv, err := encodeVariant(img, w, h, webpMime); err == nil && len(wv.Data) < len(nv.Data) {
			wv.Kind = strconv.Itoa(w) + webpKindExt
			variants = append(variants, wv)
		}
	}
	// Dedicated social preview: center-cropped to 1200x630. GIFs are preserved
	// without derivatives, so skip social for animated images.
	if format != "gif" {
		if social, err := socialVariantFor(img, cfg, format, FocalPoint{X: 0.5, Y: 0.5}); err == nil {
			variants = append(variants, social)
		}
	}
	return &Processed{Original: data, Format: format, Width: cfg.Width, Height: cfg.Height, Variants: variants}, nil
}

// webpKindExt marks a WebP derivative in its variant kind/URL slug, e.g. "480.webp".
const webpKindExt = ".webp"

// responsiveMimeFor picks the native responsive-variant format for a source.
// JPEG stays JPEG (photographic content); PNG stays PNG so transparency
// survives in browsers without WebP; WebP sources keep WebP derivatives
// instead of the old lossy-decode-then-PNG path that inflated transfer.
func responsiveMimeFor(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return webpMime
	default:
		return variantMime
	}
}

// socialVariantMimeFor picks the Open Graph preview format. Crawlers fetch
// og:image without content negotiation and have uneven WebP support, so the
// social derivative is always encoded as JPEG or PNG even for WebP sources.
func socialVariantMimeFor(format string) string {
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

// socialVariantFor crops src to the 1200x630 aspect using the focal point and
// rescales it to the social preview size. It never mutates the original bytes.
func socialVariantFor(src image.Image, cfg image.Config, format string, focal FocalPoint) (VariantBytes, error) {
	cropped := cropWithFocal(src, socialWidth, socialHeight, focal)
	dst := image.NewRGBA(image.Rect(0, 0, socialWidth, socialHeight))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), cropped, cropped.Bounds(), draw.Src, nil)

	var buf bytes.Buffer
	mime := socialVariantMimeFor(format)
	switch mime {
	case "image/png":
		if err := png.Encode(&buf, dst); err != nil {
			return VariantBytes{}, fmt.Errorf("media: encode social png: %w", err)
		}
	case "image/jpeg":
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return VariantBytes{}, fmt.Errorf("media: encode social jpeg: %w", err)
		}
	default:
		return VariantBytes{}, fmt.Errorf("media: unsupported social variant mime %q", mime)
	}
	return VariantBytes{Kind: socialKind, Width: socialWidth, Height: socialHeight, Mime: mime, Data: buf.Bytes()}, nil
}

// cropWithFocal returns the largest region of src that matches the target
// aspect, positioned around the focal point (0..1). Out-of-range focal values
// are clamped to the image bounds so a future UI can freely persist 0..1.
func cropWithFocal(src image.Image, targetW, targetH int, focal FocalPoint) image.Image {
	b := src.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	targetAspect := float64(targetW) / float64(targetH)
	srcAspect := float64(srcW) / float64(srcH)

	var cropW, cropH int
	if srcAspect > targetAspect {
		// Source is wider than target: crop width.
		cropH = srcH
		cropW = int(float64(cropH)*targetAspect + 0.5)
		if cropW < 1 {
			cropW = 1
		}
		if cropW > srcW {
			cropW = srcW
		}
	} else {
		// Source is taller (or equal): crop height.
		cropW = srcW
		cropH = int(float64(cropW)/targetAspect + 0.5)
		if cropH < 1 {
			cropH = 1
		}
		if cropH > srcH {
			cropH = srcH
		}
	}

	fx := focal.X
	if fx < 0 {
		fx = 0
	}
	if fx > 1 {
		fx = 1
	}
	fy := focal.Y
	if fy < 0 {
		fy = 0
	}
	if fy > 1 {
		fy = 1
	}

	x0 := b.Min.X + int(float64(srcW-cropW)*fx+0.5)
	y0 := b.Min.Y + int(float64(srcH-cropH)*fy+0.5)
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x0+cropW > b.Max.X {
		x0 = b.Max.X - cropW
	}
	if y0+cropH > b.Max.Y {
		y0 = b.Max.Y - cropH
	}
	return src.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(image.Rect(x0, y0, x0+cropW, y0+cropH))
}

// GenerateSocialVariant returns a 1200x630 derivative for an already stored
// image. It is exposed for regenerating the social variant after a focal-point
// change or for backfilling older assets. Caller should store the returned
// bytes as kind "social".
func GenerateSocialVariant(data []byte, focal FocalPoint) ([]byte, string, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("media: cannot read image: %w", err)
	}
	if format == "gif" {
		return nil, "", fmt.Errorf("media: social variant not generated for GIF")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("media: cannot decode image: %w", err)
	}
	v, err := socialVariantFor(img, cfg, format, focal)
	if err != nil {
		return nil, "", err
	}
	return v.Data, v.Mime, nil
}

// encodeVariant resizes src to w×h and encodes it. The encoder switch is the
// single place a new derivative format is added.
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
	case webpMime:
		if err := webp.Encode(&buf, dst, nil); err != nil {
			return VariantBytes{}, fmt.Errorf("media: encode webp: %w", err)
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
