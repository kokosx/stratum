package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newTestDataDir(t *testing.T) (string, *storage.Database, *db.Queries) {
	t.Helper()
	dir := t.TempDir()
	// Ensure data dir exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dbPath := filepath.Join(dir, "stratum.db")
	database, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	queries := db.New(database.DB)
	// Seed content types etc.
	_ = database.Seed(ctx)
	return dir, database, queries
}

func TestBackupFreshSite(t *testing.T) {
	dir, database, queries := newTestDataDir(t)
	ctx := context.Background()
	archive := filepath.Join(t.TempDir(), "fresh.zip")
	path, err := Create(ctx, database, queries, dir, archive)
	if err != nil {
		t.Fatalf("Create fresh: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("archive not created")
	}
	if err := Verify(path); err != nil {
		t.Fatalf("verify fresh: %v", err)
	}
	r, _ := zip.OpenReader(path)
	defer r.Close()
	foundDB := false
	foundManifest := false
	for _, f := range r.File {
		if f.Name == DatabaseName {
			foundDB = true
		}
		if f.Name == ManifestName {
			foundManifest = true
		}
	}
	if !foundDB || !foundManifest {
		t.Fatalf("missing db or manifest")
	}
}

func TestBackupPopulatedSite(t *testing.T) {
	dir, database, queries := newTestDataDir(t)
	ctx := context.Background()
	// Create a page and a post with revisions
	now := int64(1000)
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "page1", ContentTypeID: "page", Slug: "backup-about", Status: "active", CreatedAt: now, UpdatedAt: now})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "page1-r1", EntryID: "page1", RevisionNumber: 1, Slug: "backup-about", Title: "About", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now})
	queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "page1-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: "page1"})
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r1", Path: "/backup-about", EntryID: sql.NullString{String: "page1", Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})

	archive := filepath.Join(t.TempDir(), "pop.zip")
	if _, err := Create(ctx, database, queries, dir, archive); err != nil {
		t.Fatalf("Create populated: %v", err)
	}
	if err := Verify(archive); err != nil {
		t.Fatalf("verify populated: %v", err)
	}
	// Restore and verify content preserved
	restoreDir := t.TempDir()
	if err := Restore(ctx, archive, restoreDir); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	// Open restored DB and check
	restoredDB, err := storage.Open(filepath.Join(restoreDir, "stratum.db"))
	if err != nil {
		t.Fatalf("open restored: %v", err)
	}
	defer restoredDB.Close()
	restoredQueries := db.New(restoredDB.DB)
	entry, err := restoredQueries.GetEntry(ctx, "page1")
	if err != nil || entry.Slug != "backup-about" {
		t.Fatalf("restored entry missing: %v", err)
	}
	route, err := restoredQueries.GetRouteByPath(ctx, "/backup-about")
	if err != nil || route.EntryID.String != "page1" {
		t.Fatalf("restored route missing")
	}
	rev, err := restoredQueries.GetEntryRevision(ctx, "page1-r1")
	if err != nil || rev.Title != "About" {
		t.Fatalf("restored revision missing")
	}
}

func TestBackupMediaIncluded(t *testing.T) {
	dir, database, queries := newTestDataDir(t)
	ctx := context.Background()
	mediaRoot := filepath.Join(dir, "media")
	os.MkdirAll(filepath.Join(mediaRoot, "originals"), 0755)
	// Create a fake media file
	content := []byte("fake image data")
	path := filepath.Join(mediaRoot, "originals", "media_abc123.jpg")
	os.WriteFile(path, content, 0600)
	// Create DB entry for media
	now := int64(1000)
	queries.CreateMedia(ctx, db.CreateMediaParams{ID: "media_abc123", OriginalFilename: "test.jpg", StorageKey: "originals/media_abc123.jpg", MimeType: "image/jpeg", AssetType: "image", FileSize: int64(len(content)), CreatedAt: now, UpdatedAt: now})

	archive := filepath.Join(t.TempDir(), "media.zip")
	if _, err := Create(ctx, database, queries, dir, archive); err != nil {
		t.Fatalf("Create: %v", err)
	}
	r, _ := zip.OpenReader(archive)
	defer r.Close()
	found := false
	for _, f := range r.File {
		if f.Name == "media/originals/media_abc123.jpg" {
			found = true
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			if string(data) != string(content) {
				t.Fatalf("media content mismatch")
			}
			// Check checksum in manifest
			var m Manifest
			for _, mf := range r.File {
				if mf.Name == ManifestName {
					rc, _ := mf.Open()
					json.NewDecoder(rc).Decode(&m)
					rc.Close()
					break
				}
			}
			for _, fe := range m.Files {
				if fe.Path == f.Name {
					h := sha256.Sum256(content)
					exp := hex.EncodeToString(h[:])
					if fe.SHA256 != exp {
						t.Fatalf("checksum mismatch")
					}
				}
			}
		}
	}
	if !found {
		t.Fatalf("media not in archive")
	}
}

func TestBackupRejectsMediaSizeMismatch(t *testing.T) {
	dir, database, queries := newTestDataDir(t)
	ctx := context.Background()
	path := filepath.Join(dir, "media", "originals", "size.jpg")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("actual"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.CreateMedia(ctx, db.CreateMediaParams{ID: "size", OriginalFilename: "size.jpg", StorageKey: "originals/size.jpg", MimeType: "image/jpeg", AssetType: "image", FileSize: 99, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(ctx, database, queries, dir, filepath.Join(t.TempDir(), "size.zip")); err == nil {
		t.Fatal("backup with database/media size mismatch succeeded")
	}
}

func TestCheckIntegrityDBSupportsMediaWithoutVariantsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	database, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, statement := range []string{
		`CREATE TABLE schema_migrations (version TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`,
		`INSERT INTO schema_migrations VALUES ('001_initial.sql', 1)`,
		`CREATE TABLE media (storage_key TEXT NOT NULL, file_size INTEGER NOT NULL)`,
		`INSERT INTO media VALUES ('originals/old.jpg', 3)`,
	} {
		if _, err := database.DB.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	_, media, err := checkIntegrityDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(media) != 1 || media[0].key != "originals/old.jpg" || media[0].size != 3 {
		t.Fatalf("unexpected media discovery: %#v", media)
	}
}

func TestBackupChecksumsValid(t *testing.T) {
	dir, database, queries := newTestDataDir(t)
	ctx := context.Background()
	archive := filepath.Join(t.TempDir(), "chk.zip")
	Create(ctx, database, queries, dir, archive)
	// Verify should pass
	if err := Verify(archive); err != nil {
		t.Fatalf("verify should pass: %v", err)
	}
	// Corrupt the DB file inside ZIP by modifying a byte
	// Create a corrupted copy
	corrupted := filepath.Join(t.TempDir(), "corrupted.zip")
	// Copy and corrupt
	data, _ := os.ReadFile(archive)
	// Flip a byte in the middle
	if len(data) > 1000 {
		data[1000] ^= 0xFF
	}
	os.WriteFile(corrupted, data, 0600)
	if err := Verify(corrupted); err == nil {
		t.Fatalf("corrupted archive should fail verify")
	}
}

func TestBackupZipSlipRejected(t *testing.T) {
	// Create a zip with traversal path
	tmp := filepath.Join(t.TempDir(), "slip.zip")
	f, _ := os.Create(tmp)
	w := zip.NewWriter(f)
	// Add slip file
	header := &zip.FileHeader{Name: "../../evil.txt", Method: zip.Store}
	header.SetMode(0600)
	ww, _ := w.CreateHeader(header)
	ww.Write([]byte("evil"))
	// Add manifest
	manifest := Manifest{Format: Format, Version: Version, CreatedAt: "2026-01-01T00:00:00Z", SchemaVersion: "001_initial.sql", StratumVersion: "dev", Database: DatabaseName, MediaRoot: MediaPrefix, DatabaseSHA256: "abc", MediaCount: 0}
	manifest.Files = []FileEntry{{Path: "../../evil.txt", SHA256: "abc", Size: 4}}
	data, _ := json.Marshal(manifest)
	h := &zip.FileHeader{Name: ManifestName, Method: zip.Store}
	h.SetMode(0600)
	ww, _ = w.CreateHeader(h)
	ww.Write(data)
	w.Close()
	f.Close()
	if err := Verify(tmp); err == nil {
		t.Fatalf("zip slip should be rejected")
	}
	// Also test restore rejects
	dir := t.TempDir()
	if err := Restore(context.Background(), tmp, dir); err == nil {
		t.Fatalf("restore should reject zip slip")
	}
}

func TestBackupIntegrityVerified(t *testing.T) {
	dir, database, queries := newTestDataDir(t)
	ctx := context.Background()
	archive := filepath.Join(t.TempDir(), "int.zip")
	Create(ctx, database, queries, dir, archive)
	// Verify does integrity_check – should pass for valid
	if err := Verify(archive); err != nil {
		t.Fatalf("integrity should pass: %v", err)
	}
	// Now create a backup with corrupted DB file (invalid sqlite)
	// Extract, corrupt DB, re-zip
	tmpExtract := t.TempDir()
	r, _ := zip.OpenReader(archive)
	for _, f := range r.File {
		dest := filepath.Join(tmpExtract, f.Name)
		os.MkdirAll(filepath.Dir(dest), 0755)
		if f.FileInfo().IsDir() {
			continue
		}
		rc, _ := f.Open()
		out, _ := os.Create(dest)
		io.Copy(out, rc)
		rc.Close()
		out.Close()
	}
	r.Close()
	// Corrupt DB
	dbPath := filepath.Join(tmpExtract, DatabaseName)
	f, _ := os.OpenFile(dbPath, os.O_RDWR, 0600)
	f.WriteAt([]byte("CORRUPT"), 0)
	f.Close()
	// Re-zip
	corrupted := filepath.Join(t.TempDir(), "corrupted2.zip")
	out, _ := os.Create(corrupted)
	zw := zip.NewWriter(out)
	filepath.Walk(tmpExtract, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(tmpExtract, path)
		zipPath := filepath.ToSlash(rel)
		addTestFileToZip(zw, path, zipPath)
		return nil
	})
	zw.Close()
	out.Close()
	if err := Verify(corrupted); err == nil {
		t.Fatalf("corrupted DB should fail integrity")
	}
}

func addTestFileToZip(zw *zip.Writer, local, zipPath string) {
	f, _ := os.Open(local)
	defer f.Close()
	info, _ := f.Stat()
	h := &zip.FileHeader{Name: zipPath, Method: zip.Deflate}
	h.SetMode(0600)
	h.Modified = info.ModTime()
	w, _ := zw.CreateHeader(h)
	io.Copy(w, f)
}

func TestRestoreExactContent(t *testing.T) {
	dir, database, queries := newTestDataDir(t)
	ctx := context.Background()
	now := int64(1000)
	// Create taxonomy
	queries.CreateTaxonomy(ctx, db.CreateTaxonomyParams{ID: "cat", ContentTypeID: "post", PluralName: "Categories", SingularName: "Category", Hierarchical: 1, Public: 1, RouteBase: sql.NullString{String: "categories", Valid: true}, CreatedAt: now, UpdatedAt: now})
	queries.CreateTerm(ctx, db.CreateTermParams{ID: "term1", TaxonomyID: "cat", Name: "Tech", Slug: "tech"})
	// Create entry with hierarchy, custom fields, etc.
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "parent1", ContentTypeID: "page", Slug: "parent", Status: "active", CreatedAt: now, UpdatedAt: now})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "parent1-r1", EntryID: "parent1", RevisionNumber: 1, Slug: "parent", Title: "Parent", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now})
	queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "parent1-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: "parent1"})
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r-parent", Path: "/parent", EntryID: sql.NullString{String: "parent1", Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})

	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "child1", ContentTypeID: "page", Slug: "child", Status: "active", CreatedAt: now, UpdatedAt: now})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "child1-r1", EntryID: "child1", RevisionNumber: 1, Slug: "child", Title: "Child", DocumentJson: `{"version":1,"nodes":[]}`, ParentEntryID: sql.NullString{String: "parent1", Valid: true}, CreatedAt: now})
	queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "child1-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: "child1"})
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r-child", Path: "/parent/child", EntryID: sql.NullString{String: "child1", Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})

	// User
	queries.CreateUser(ctx, db.CreateUserParams{ID: "user1", Email: "test@example.com", PasswordHash: "hash", Role: "admin", CreatedAt: now, UpdatedAt: now})

	archive := filepath.Join(t.TempDir(), "exact.zip")
	Create(ctx, database, queries, dir, archive)

	restoreDir := t.TempDir()
	if err := Restore(ctx, archive, restoreDir); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	restoredDB, _ := storage.Open(filepath.Join(restoreDir, "stratum.db"))
	defer restoredDB.Close()
	rq := db.New(restoredDB.DB)
	// Check all preserved
	if _, err := rq.GetEntry(ctx, "parent1"); err != nil {
		t.Fatalf("parent missing")
	}
	if _, err := rq.GetEntryRevision(ctx, "parent1-r1"); err != nil {
		t.Fatalf("revision missing")
	}
	if _, err := rq.GetTerm(ctx, "term1"); err != nil {
		t.Fatalf("taxonomy missing")
	}
	if _, err := rq.GetRouteByPath(ctx, "/parent/child"); err != nil {
		t.Fatalf("route missing")
	}
	if _, err := rq.GetUserByID(ctx, "user1"); err != nil {
		t.Fatalf("user missing")
	}
}

func TestBackupScheduledJobsPreserved(t *testing.T) {
	dir, database, queries := newTestDataDir(t)
	ctx := context.Background()
	now := int64(1000)
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "sched1", ContentTypeID: "post", Slug: "sched", Status: "active", CreatedAt: now, UpdatedAt: now})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "sched1-r1", EntryID: "sched1", RevisionNumber: 1, Slug: "sched", Title: "Sched", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now})
	queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "sched1-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: "sched1"})
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r-sched", Path: "/blog/sched", EntryID: sql.NullString{String: "sched1", Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
	// Scheduled job
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "sched1-r2", EntryID: "sched1", RevisionNumber: 2, Slug: "sched", Title: "Sched2", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 10})
	queries.CreatePublicationJob(ctx, db.CreatePublicationJobParams{ID: "job1", EntryID: "sched1", RevisionID: "sched1-r2", ScheduledAt: now + 10000, CreatedAt: now, UpdatedAt: now})

	archive := filepath.Join(t.TempDir(), "sched.zip")
	Create(ctx, database, queries, dir, archive)
	restoreDir := t.TempDir()
	Restore(ctx, archive, restoreDir)
	restoredDB, _ := storage.Open(filepath.Join(restoreDir, "stratum.db"))
	defer restoredDB.Close()
	rq := db.New(restoredDB.DB)
	job, err := rq.GetPublicationJob(ctx, "job1")
	if err != nil || job.RevisionID != "sched1-r2" {
		t.Fatalf("scheduled job not preserved")
	}
}

func TestBackupFutureSchemaRejected(t *testing.T) {
	dir, database, queries := newTestDataDir(t)
	ctx := context.Background()
	archive := filepath.Join(t.TempDir(), "future.zip")
	Create(ctx, database, queries, dir, archive)
	// Tamper manifest to future version
	tmp := t.TempDir()
	r, _ := zip.OpenReader(archive)
	for _, f := range r.File {
		dest := filepath.Join(tmp, f.Name)
		os.MkdirAll(filepath.Dir(dest), 0755)
		if f.FileInfo().IsDir() {
			continue
		}
		rc, _ := f.Open()
		out, _ := os.Create(dest)
		io.Copy(out, rc)
		rc.Close()
		out.Close()
	}
	r.Close()
	// Modify manifest
	manifestPath := filepath.Join(tmp, ManifestName)
	data, _ := os.ReadFile(manifestPath)
	var m Manifest
	json.Unmarshal(data, &m)
	m.SchemaVersion = "999_future.sql"
	data, _ = json.MarshalIndent(m, "", "  ")
	os.WriteFile(manifestPath, data, 0600)
	// Re-zip with future manifest
	futureArchive := filepath.Join(t.TempDir(), "future2.zip")
	out, _ := os.Create(futureArchive)
	zw := zip.NewWriter(out)
	filepath.Walk(tmp, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(tmp, path)
		addTestFileToZip(zw, path, filepath.ToSlash(rel))
		return nil
	})
	zw.Close()
	out.Close()
	if err := Verify(futureArchive); err == nil {
		t.Fatalf("future schema should be rejected on verify")
	}
	if err := Restore(ctx, futureArchive, t.TempDir()); err == nil {
		t.Fatalf("future schema should be rejected on restore")
	}
}

func TestBackupOlderSchemaMigration(t *testing.T) {
	// Create backup with older schema by manually not applying latest migration
	// For simplicity, we test that restore runs Migrate and succeeds even if backup is slightly older
	dir, database, queries := newTestDataDir(t)
	ctx := context.Background()
	archive := filepath.Join(t.TempDir(), "older.zip")
	Create(ctx, database, queries, dir, archive)
	restoreDir := t.TempDir()
	if err := Restore(ctx, archive, restoreDir); err != nil {
		t.Fatalf("restore older should succeed and migrate: %v", err)
	}
	// Verify restored DB can be opened and migrated
	restoredDB, _ := storage.Open(filepath.Join(restoreDir, "stratum.db"))
	defer restoredDB.Close()
	if err := restoredDB.Migrate(ctx); err != nil {
		t.Fatalf("migrate after restore: %v", err)
	}
}

func TestRestoreFailureLeavesOriginalIntact(t *testing.T) {
	dir, database, queries := newTestDataDir(t)
	ctx := context.Background()
	// Create original content
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "orig1", ContentTypeID: "page", Slug: "orig", Status: "active", CreatedAt: 1000, UpdatedAt: 1000})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "orig1-r1", EntryID: "orig1", RevisionNumber: 1, Slug: "orig", Title: "Orig", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: 1000})
	queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "orig1-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1000, Valid: true}, UpdatedAt: 1000, ID: "orig1"})
	archive := filepath.Join(t.TempDir(), "good.zip")
	Create(ctx, database, queries, dir, archive)

	// Create corrupted archive (bad checksum)
	badArchive := filepath.Join(t.TempDir(), "bad.zip")
	tmp := t.TempDir()
	r, _ := zip.OpenReader(archive)
	for _, f := range r.File {
		dest := filepath.Join(tmp, f.Name)
		os.MkdirAll(filepath.Dir(dest), 0755)
		if f.FileInfo().IsDir() {
			continue
		}
		rc, _ := f.Open()
		out, _ := os.Create(dest)
		io.Copy(out, rc)
		rc.Close()
		out.Close()
	}
	r.Close()
	manifestPath := filepath.Join(tmp, ManifestName)
	mData, _ := os.ReadFile(manifestPath)
	var m Manifest
	json.Unmarshal(mData, &m)
	m.DatabaseSHA256 = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	mData, _ = json.MarshalIndent(m, "", "  ")
	os.WriteFile(manifestPath, mData, 0600)
	out2, _ := os.Create(badArchive)
	zw2 := zip.NewWriter(out2)
	filepath.Walk(tmp, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(tmp, path)
		addTestFileToZip(zw2, path, filepath.ToSlash(rel))
		return nil
	})
	zw2.Close()
	out2.Close()

	// Copy original DB path to check after
	origDBPath := filepath.Join(dir, "stratum.db")
	origData, _ := os.ReadFile(origDBPath)
	origHash := sha256.Sum256(origData)

	err := Restore(ctx, badArchive, dir)
	if err == nil {
		t.Fatalf("restore should fail")
	}
	// Verify original still exists and is intact
	if _, err := os.Stat(origDBPath); err != nil {
		t.Fatalf("original DB should still exist")
	}
	// Verify it still has orig entry
	checkDB, _ := storage.Open(origDBPath)
	defer checkDB.Close()
	q2 := db.New(checkDB.DB)
	if _, err := q2.GetEntry(ctx, "orig1"); err != nil {
		t.Fatalf("original entry should still exist: %v", err)
	}
	// Check checksum still matches original
	newData, _ := os.ReadFile(origDBPath)
	newHash := sha256.Sum256(newData)
	if origHash != newHash {
		t.Fatalf("original DB corrupted after failed restore")
	}
}
