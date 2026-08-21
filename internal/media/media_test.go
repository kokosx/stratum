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
	if len(processed.Variants) != 2 {
		t.Fatalf("variants = %d, want 2 (480, 768)", len(processed.Variants))
	}
	if processed.Variants[0].Width != 480 || processed.Variants[1].Width != 768 {
		t.Fatalf("variant widths = %d,%d, want 480,768", processed.Variants[0].Width, processed.Variants[1].Width)
	}
	// PNG sources keep PNG variants so transparency survives.
	if processed.Variants[0].Mime != "image/png" {
		t.Fatalf("variant mime = %q, want image/png", processed.Variants[0].Mime)
	}
}

func TestProcessImageNeverUpscales(t *testing.T) {
	processed, err := ProcessImage(testPNG(t, 300, 200))
	if err != nil {
		t.Fatalf("ProcessImage() error = %v", err)
	}
	if len(processed.Variants) != 0 {
		t.Fatalf("small image generated %d variants, want 0", len(processed.Variants))
	}
	if processed.Width != 300 || processed.Height != 200 {
		t.Fatalf("dimensions = %dx%d, want 300x200", processed.Width, processed.Height)
	}
}

func TestProcessImageRejectsUnsupportedAndSvg(t *testing.T) {
	if _, err := ProcessImage([]byte("this is not an image")); err == nil {
		t.Fatal("ProcessImage accepted garbage bytes")
	}
	if _, err := ProcessImage([]byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>")); err == nil {
		t.Fatal("ProcessImage accepted an SVG")
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
	if len(asset.Variants) != 2 { // 480 and 768
		t.Fatalf("variants = %d, want 2", len(asset.Variants))
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
	if view.Src != "/media/"+asset.ID+"/768" {
		t.Fatalf("Src = %q, want /media/%s/768", view.Src, asset.ID)
	}
	if !strings.Contains(view.SrcSet, "/media/"+asset.ID+"/480 480w") || !strings.Contains(view.SrcSet, "/media/"+asset.ID+"/768 768w") {
		t.Fatalf("SrcSet = %q", view.SrcSet)
	}
	if view.Width != 800 || view.Height != 600 {
		t.Fatalf("view dims = %dx%d", view.Width, view.Height)
	}
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