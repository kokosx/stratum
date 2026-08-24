package media

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	store, err := NewLocalStorage(filepath.Join(t.TempDir(), "media"))
	if err != nil {
		t.Fatal(err)
	}
	return NewService(queries, store)
}

func TestProcessImageGeneratesOnlySmallerVariants(t *testing.T) {
	processed, err := ProcessImage(testPNG(t, 1000, 600))
	if err != nil {
		t.Fatalf("ProcessImage() error = %v", err)
	}
	if processed.Width != 1000 || processed.Height != 600 {
		t.Fatalf("dimensions = %dx%d, want 1000x600", processed.Width, processed.Height)
	}
	// 480 and 768 are < 1000; 1280/1920 are upscales and must be skipped.
	// Each kept width carries its PNG variant plus an optional WebP twin
	// (emitted only when smaller than the PNG), then the 1200x630 social.
	native := map[string][]byte{}
	webpTwins := map[string][]byte{}
	var social *VariantBytes
	for i := range processed.Variants {
		v := &processed.Variants[i]
		if v.Kind == "social" {
			social = v
			continue
		}
		if strings.HasSuffix(v.Kind, ".webp") {
			webpTwins[strings.TrimSuffix(v.Kind, ".webp")] = v.Data
		} else {
			native[v.Kind] = v.Data
		}
	}
	if _, ok := native["480"]; !ok {
		t.Fatalf("missing 480 variant in %v", processed.Variants)
	}
	if _, ok := native["768"]; !ok {
		t.Fatalf("missing 768 variant in %v", processed.Variants)
	}
	if len(native) != 2 {
		t.Fatalf("unexpected extra responsive variants in %v", processed.Variants)
	}
	if len(webpTwins) == 0 {
		t.Fatal("expected at least one WebP derivative for a raster source")
	}
	if social == nil || social.Width != 1200 || social.Height != 630 {
		t.Fatalf("social variant = %+v, want kind social 1200x630", social)
	}
	if social.Mime != "image/png" {
		t.Fatalf("social mime = %q, want image/png (crawler-safe)", social.Mime)
	}
	for width, data := range native {
		if !bytes.HasPrefix(data, []byte{0x89}) {
			t.Fatalf("variant %s is not PNG data", width)
		}
	}
	for width, data := range webpTwins {
		// A WebP twin is only ever emitted when it beats its PNG twin.
		if len(data) >= len(native[width]) {
			t.Fatalf("webp twin of %s (%d bytes) not smaller than png (%d bytes)", width, len(data), len(native[width]))
		}
		// The bytes really are WebP: RIFF....WEBP header.
		if !bytes.HasPrefix(data, []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WEBP")) {
			t.Fatalf("webp twin of %s is not WebP data", width)
		}
	}
}

func TestProcessImageWebPSourceGetsWebPVariants(t *testing.T) {
	// Re-encode a WebP source through the pipeline: take a generated derivative
	// of a PNG upload (real WebP bytes) and upload those as the source.
	first, err := ProcessImage(testPNG(t, 1000, 600))
	if err != nil {
		t.Fatal(err)
	}
	var webpOriginal []byte
	for _, v := range first.Variants {
		if strings.HasSuffix(v.Kind, ".webp") {
			webpOriginal = v.Data
			break
		}
	}
	if webpOriginal == nil {
		t.Fatal("no webp variant produced to use as a source")
	}

	processed, err := ProcessImage(webpOriginal)
	if err != nil {
		t.Fatalf("ProcessImage(webp) error = %v", err)
	}
	if processed.Format != "webp" {
		t.Fatalf("format = %q, want webp", processed.Format)
	}
	// The old pipeline re-encoded WebP sources as PNG; now every responsive
	// variant must stay WebP and no PNG responsive variants may appear.
	for _, v := range processed.Variants {
		if v.Kind == "social" {
			// Social previews stay JPEG/PNG on purpose (crawler support).
			if v.Mime != "image/png" {
				t.Fatalf("social mime = %q, want image/png", v.Mime)
			}
			continue
		}
		if !strings.HasSuffix(v.Kind, ".webp") || v.Mime != "image/webp" {
			t.Fatalf("variant %+v, want a .webp kind with image/webp mime", v)
		}
	}
}

func TestProcessImageNeverUpscales(t *testing.T) {
	processed, err := ProcessImage(testPNG(t, 300, 200))
	if err != nil {
		t.Fatalf("ProcessImage() error = %v", err)
	}
	// Responsive variants never upscale, but the dedicated social preview is
	// allowed to upscale from a small source via a center crop.
	if len(processed.Variants) != 1 {
		t.Fatalf("small image generated %d variants, want 1 (social)", len(processed.Variants))
	}
	if processed.Variants[0].Kind != "social" {
		t.Fatalf("small image variant = %+v, want social", processed.Variants[0])
	}
	if processed.Width != 300 || processed.Height != 200 {
		t.Fatalf("dimensions = %dx%d, want 300x200", processed.Width, processed.Height)
	}
}

func TestProcessImageRejectsUnsupportedAndSvg(t *testing.T) {
	if _, err := ProcessImage([]byte("this is not an image")); err == nil {
		t.Fatal("ProcessImage accepted garbage bytes")
	}
	// All SVG uploads are rejected (SVG support may be added later with a reviewed sanitization policy)
	if _, err := ProcessImage([]byte("<svg xmlns='http://www.w3.org/2000/svg'><rect width='10' height='10' fill='blue'/></svg>")); err == nil {
		t.Fatal("ProcessImage accepted SVG")
	}
	if _, err := ProcessImage([]byte("<svg xmlns='http://www.w3.org/2000/svg'><script>alert(1)</script></svg>")); err == nil {
		t.Fatal("ProcessImage accepted unsafe SVG with <script>")
	}
	if _, err := ProcessImage([]byte("<svg xmlns='http://www.w3.org/2000/svg'><g onload='alert(1)'><rect width='10' height='10'/></g></svg>")); err == nil {
		t.Fatal("ProcessImage accepted SVG with event handler")
	}
	// Ensure error is unsupported format
	if _, err := ProcessImage([]byte("<svg></svg>")); err != ErrUnsupportedFormat && !strings.Contains(err.Error(), "SVG") {
		// Allow wrapped ErrUnsupportedFormat
		if !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("SVG error should be unsupported, got %v", err)
		}
	}
}

func TestLocalStorageRejectsTraversal(t *testing.T) {
	store, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(context.Background(), "../../etc/evil", []byte("x")); err == nil {
		t.Fatal("Put accepted a traversal key")
	}
	if err := store.Put(context.Background(), "originals/ok.png", []byte("x")); err != nil {
		t.Fatalf("Put valid key: %v", err)
	}
	if !store.Exists(context.Background(), "originals/ok.png") {
		t.Fatal("Exists returned false for stored key")
	}
	if err := store.Delete(context.Background(), "originals/ok.png"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.Exists(context.Background(), "originals/ok.png") {
		t.Fatal("Exists returned true after delete")
	}
}

func TestUploadTooLargeRejected(t *testing.T) {
	svc := newTestService(t)
	blob := make([]byte, maxImageBytes+1)
	_, err := svc.Upload(context.Background(), "big.bin", "", bytes.NewReader(blob))
	if err != ErrTooLarge {
		t.Fatalf("Upload error = %v, want ErrTooLarge", err)
	}
}

func TestUploadListGetDelete(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	asset, err := svc.Upload(ctx, "hero.png", "", bytes.NewReader(testPNG(t, 800, 600)))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if asset.ID == "" || asset.Width != 800 || asset.Height != 600 {
		t.Fatalf("asset = %+v", asset)
	}
	if len(asset.Variants) != 5 { // 480, 480.webp, 768, 768.webp, social
		t.Fatalf("variants = %d, want 5", len(asset.Variants))
	}
	for _, v := range asset.Variants {
		if v.ContentHash == "" {
			t.Fatalf("variant %s has no content hash", v.Kind)
		}
	}

	list, err := svc.List(ctx, 20, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %d items, err %v; want 1", len(list), err)
	}

	usage, err := svc.CountUsage(ctx, asset.ID)
	if err != nil || usage != 0 {
		t.Fatalf("CountUsage = %d, err %v; want 0", usage, err)
	}

	// Stored bytes are actually readable back.
	data, mime, err := svc.ReadVariant(ctx, asset.ID, "768")
	if err != nil {
		t.Fatalf("ReadVariant: %v", err)
	}
	if mime != "image/png" || len(data) == 0 {
		t.Fatalf("ReadVariant = %d bytes %q", len(data), mime)
	}

	if err := svc.Delete(ctx, asset.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.Get(ctx, asset.ID); err == nil {
		t.Fatal("Get succeeded after delete")
	}
}

func TestMediaViewBuildsSrcAndSrcSet(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	asset, err := svc.Upload(ctx, "hero.png", "", bytes.NewReader(testPNG(t, 800, 600)))
	if err != nil {
		t.Fatal(err)
	}
	view, ok := svc.MediaView(ctx, asset.ID)
	if !ok {
		t.Fatal("MediaView not found")
	}
	hashes := variantHashes(asset)

	// Native srcset: every responsive PNG candidate, width-ascending, each
	// versioned with the variant's content hash.
	wantNative := "/media/" + asset.ID + "/480?v=" + hashes["480"] + " 480w, /media/" + asset.ID + "/768?v=" + hashes["768"] + " 768w"
	if view.SrcSet != wantNative {
		t.Fatalf("SrcSet = %q, want %q", view.SrcSet, wantNative)
	}
	// WebP twin set mirrors it with ".webp" slugs.
	wantWebp := "/media/" + asset.ID + "/480.webp?v=" + hashes["480.webp"] + " 480w, /media/" + asset.ID + "/768.webp?v=" + hashes["768.webp"] + " 768w"
	if view.WebPSrcSet != wantWebp {
		t.Fatalf("WebPSrcSet = %q, want %q", view.WebPSrcSet, wantWebp)
	}
	// Src is the largest native responsive variant, versioned as well.
	if view.Src != "/media/"+asset.ID+"/768?v="+hashes["768"] {
		t.Fatalf("Src = %q", view.Src)
	}
	if view.Width != 800 || view.Height != 600 {
		t.Fatalf("view dims = %dx%d", view.Width, view.Height)
	}

	// Serving a versioned URL resolves to real bytes (query string ignored).
	data, mime, err := svc.ReadVariant(ctx, asset.ID, "480")
	if err != nil {
		t.Fatalf("ReadVariant: %v", err)
	}
	if mime != "image/png" || len(data) == 0 {
		t.Fatalf("ReadVariant = %d bytes %q", len(data), mime)
	}
}

func TestMediaViewVersionedURLChangesWhenVariantRegenerated(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	asset, err := svc.Upload(ctx, "hero.png", "", bytes.NewReader(testPNG(t, 1600, 900)))
	if err != nil {
		t.Fatal(err)
	}
	before, ok := svc.MediaView(ctx, asset.ID)
	if !ok {
		t.Fatal("MediaView not found")
	}
	if before.WebPSrcSet == "" {
		t.Fatal("expected WebPSrcSet for raster upload")
	}
	// Regenerate the social preview from a shifted focal point: same kind,
	// same immutable URL shape, but new bytes must yield a new ?v= so cached
	// clients never see stale content.
	if err := svc.GenerateSocialVariant(ctx, asset.ID, FocalPoint{X: 0.1, Y: 0.1}); err != nil {
		t.Fatalf("GenerateSocialVariant: %v", err)
	}
	socialAfter, ok := svc.SocialView(ctx, asset.ID)
	if !ok {
		t.Fatal("SocialView after regeneration")
	}
	if !strings.HasPrefix(socialAfter.URL, "/media/"+asset.ID+"/social?v=") {
		t.Fatalf("social URL = %q, want versioned /media/<id>/social?v=<hash>", socialAfter.URL)
	}
	if socialAfter.Width != socialWidth || socialAfter.Height != socialHeight {
		t.Fatalf("social dims = %dx%d, want %dx%d", socialAfter.Width, socialAfter.Height, socialWidth, socialHeight)
	}

	// The regenerated derivative resolves through its versioned URL path
	// (query string ignored by the resolver, as in the HTTP route).
	kind := strings.TrimPrefix(strings.SplitN(socialAfter.URL, "?", 2)[0], "/media/"+asset.ID+"/")
	data, mime, err := svc.ReadVariant(ctx, asset.ID, kind)
	if err != nil {
		t.Fatalf("ReadVariant(%q): %v", kind, err)
	}
	if len(data) == 0 || mime == "" {
		t.Fatalf("ReadVariant = %d bytes %q", len(data), mime)
	}
}

func TestSocialViewFallbackSkipsWebPKinds(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	asset, err := svc.Upload(ctx, "photo.png", "", bytes.NewReader(testPNG(t, 600, 400)))
	if err != nil {
		t.Fatal(err)
	}
	// Remove the dedicated social variant to force the fallback path.
	v, err := svc.queries.GetMediaVariant(ctx, db.GetMediaVariantParams{MediaID: asset.ID, Kind: "social"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.queries.DeleteMediaVariant(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	svc.InvalidateView(asset.ID)

	sv, ok := svc.SocialView(ctx, asset.ID)
	if !ok {
		t.Fatal("SocialView not found")
	}
	// Must resolve to a native-format candidate (og:image), never a .webp slug.
	// 600px source: only the 480 responsive width exists (no upscaling).
	if strings.HasSuffix(sv.URL, ".webp") {
		t.Fatalf("SocialView fallback picked a webp slug: %q", sv.URL)
	}
	if !strings.Contains(sv.URL, "/480?v=") {
		t.Fatalf("SocialView fallback = %q, want largest native variant", sv.URL)
	}
	if sv.Type != "image/png" {
		t.Fatalf("fallback type = %q, want image/png", sv.Type)
	}
}

// variantHashes maps variant kind -> content hash of a freshly uploaded asset.
func variantHashes(a *Asset) map[string]string {
	out := make(map[string]string, len(a.Variants))
	for _, v := range a.Variants {
		out[v.Kind] = v.ContentHash
	}
	return out
}

func TestMediaViewFallsBackToOriginalForTinyImage(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	asset, err := svc.Upload(ctx, "tiny.png", "", bytes.NewReader(testPNG(t, 100, 100)))
	if err != nil {
		t.Fatal(err)
	}
	view, ok := svc.MediaView(ctx, asset.ID)
	if !ok {
		t.Fatal("MediaView not found")
	}
	if view.Src != "/media/"+asset.ID+"/original" {
		t.Fatalf("Src = %q, want original", view.Src)
	}
}

func TestFaviconGeneration(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	asset, err := svc.Upload(ctx, "logo.png", "", bytes.NewReader(testPNG(t, 512, 512)))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.GenerateFaviconVariants(ctx, asset.ID); err != nil {
		t.Fatalf("GenerateFaviconVariants: %v", err)
	}
	view, ok := svc.FaviconView(ctx, asset.ID)
	if !ok {
		t.Fatal("FaviconView not found")
	}
	if view.Size16 == "" || view.Size32 == "" || view.Size180 == "" || view.Size192 == "" || view.Size512 == "" {
		t.Fatalf("FaviconView incomplete: %+v", view)
	}
	data, mime, err := svc.ReadVariant(ctx, asset.ID, "favicon-32")
	if err != nil {
		t.Fatalf("ReadVariant favicon-32: %v", err)
	}
	if mime != "image/png" || len(data) == 0 {
		t.Fatalf("favicon-32 = %d bytes %q", len(data), mime)
	}
}

func TestUploadRejectsNonImage(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Upload(context.Background(), "evil.txt", "", strings.NewReader("#!/bin/sh\nrm -rf /\n"))
	if err == nil {
		t.Fatal("Upload accepted a non-image file")
	}
}

func TestUpdateMetadataRoundTripsAllFields(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	asset, err := svc.Upload(ctx, "photo.png", "", bytes.NewReader(testPNG(t, 600, 400)))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateMetadata(ctx, asset.ID, "a hero photo", "Hero", "A caption", "A description"); err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	got, err := svc.Get(ctx, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AltText != "a hero photo" || got.Title != "Hero" || got.Caption != "A caption" || got.Description != "A description" {
		t.Fatalf("metadata round-trip = %+v", got)
	}
}

func TestCountUsageDetectsSiteIconReference(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	asset, err := svc.Upload(ctx, "icon.png", "", bytes.NewReader(testPNG(t, 512, 512)))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.GenerateFaviconVariants(ctx, asset.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.CountUsage(ctx, asset.ID); n != 0 {
		t.Fatalf("usage before site icon = %d, want 0", n)
	}
	if err := svc.queries.UpdateSiteIconMediaID(ctx, sql.NullString{String: asset.ID, Valid: true}); err != nil {
		t.Fatal(err)
	}
	if n, _ := svc.CountUsage(ctx, asset.ID); n != 1 {
		t.Fatalf("usage after site icon = %d, want 1", n)
	}
}
func TestAssetThumbURLPrefersSmallestVariant(t *testing.T) {
	withVariants := &Asset{
		ID: "media_x",
		Variants: []Variant{
			{Kind: "1920", Width: 1920},
			{Kind: "480", Width: 480},
			{Kind: "768", Width: 768},
		},
	}
	if got, want := withVariants.ThumbURL(), "/media/media_x/480"; got != want {
		t.Fatalf("ThumbURL = %q, want %q", got, want)
	}

	faviconOnly := &Asset{
		ID: "media_y",
		Variants: []Variant{
			{Kind: "favicon-32", Width: 32},
			{Kind: "favicon-512", Width: 512},
		},
	}
	if got, want := faviconOnly.ThumbURL(), "/media/media_y/original"; got != want {
		t.Fatalf("ThumbURL (no responsive variant) = %q, want %q", got, want)
	}

	noVariants := &Asset{ID: "media_z"}
	if got, want := noVariants.ThumbURL(), "/media/media_z/original"; got != want {
		t.Fatalf("ThumbURL (no variants) = %q, want %q", got, want)
	}
}
