package publishing

import (
	"context"
	"database/sql"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestSchedulerExactRevisionInvariant(t *testing.T) {
	svc, sched, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	// Create/publish R1
	rev1 := createEntryWithRevision(t, queries, "e1", "post", "e1", "public", 0, "draft")
	now := int64(1000)
	if err := svc.PublishRevision(ctx, "e1", rev1, now); err != nil {
		t.Fatalf("publish R1: %v", err)
	}
	// Create R2
	now2 := int64(1010)
	rev2ID := "e1-r2"
	doc := `{"version":1,"nodes":[]}`
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2ID, EntryID: "e1", RevisionNumber: 2, Slug: "e1", Title: "R2", DocumentJson: doc, CreatedAt: now2}); err != nil {
		t.Fatalf("R2: %v", err)
	}
	// Schedule R2 at 2000
	if err := svc.Schedule(ctx, "e1", rev2ID, 2000, "", now2); err != nil {
		t.Fatalf("schedule R2: %v", err)
	}
	// Create R3 (latest draft) not scheduled
	now3 := int64(1020)
	rev3ID := "e1-r3"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev3ID, EntryID: "e1", RevisionNumber: 3, Slug: "e1", Title: "R3", DocumentJson: doc, CreatedAt: now3}); err != nil {
		t.Fatalf("R3: %v", err)
	}
	// Run scheduler at due time 2000
	if err := sched.RunDue(ctx, 2000); err != nil {
		t.Fatalf("RunDue: %v", err)
	}
	// Assert published is R2, latest is R3
	entry, _ := queries.GetEntry(ctx, "e1")
	if entry.PublishedRevisionID.String != rev2ID {
		t.Fatalf("published should be R2 %s, got %s", rev2ID, entry.PublishedRevisionID.String)
	}
	latest, _ := queries.GetLatestEntryRevision(ctx, "e1")
	if latest.ID != rev3ID {
		t.Fatalf("latest should be R3 %s, got %s", rev3ID, latest.ID)
	}
	// Public content corresponds to R2 – check via GetPublishedEntryByID
	row, err := queries.GetPublishedEntryByID(ctx, "e1")
	if err != nil {
		t.Fatalf("GetPublishedEntryByID: %v", err)
	}
	if row.RevisionID != rev2ID {
		t.Fatalf("public revision should be R2, got %s", row.RevisionID)
	}
}

func TestSchedulerBeforeDueNoPublish(t *testing.T) {
	svc, sched, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	rev1 := createEntryWithRevision(t, queries, "e2", "post", "e2", "public", 0, "draft")
	now := int64(1000)
	svc.PublishRevision(ctx, "e2", rev1, now)
	rev2ID := "e2-r2"
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2ID, EntryID: "e2", RevisionNumber: 2, Slug: "e2", Title: "R2", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 5})
	svc.Schedule(ctx, "e2", rev2ID, 5000, "", now+5)
	// Before due
	sched.RunDue(ctx, 4000)
	entry, _ := queries.GetEntry(ctx, "e2")
	if entry.PublishedRevisionID.String == rev2ID {
		t.Fatalf("should not publish before due")
	}
}

func TestSchedulerRunDueTwiceNoDuplicate(t *testing.T) {
	svc, sched, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	rev1 := createEntryWithRevision(t, queries, "e3", "post", "e3", "public", 0, "draft")
	now := int64(1000)
	svc.PublishRevision(ctx, "e3", rev1, now)
	rev2ID := "e3-r2"
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2ID, EntryID: "e3", RevisionNumber: 2, Slug: "e3", Title: "R2", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 5})
	svc.Schedule(ctx, "e3", rev2ID, 2000, "", now+5)
	sched.RunDue(ctx, 2000)
	sched.RunDue(ctx, 2000)
	entry, _ := queries.GetEntry(ctx, "e3")
	if entry.PublishedRevisionID.String != rev2ID {
		t.Fatalf("should remain R2")
	}
	jobs, _ := queries.ListPublicationJobsForEntry(ctx, "e3")
	completed := 0
	for _, j := range jobs {
		if j.Status == "completed" {
			completed++
		}
	}
	if completed != 1 {
		t.Fatalf("should have exactly one completed, got %d", completed)
	}
}

func TestSchedulerOverduePublishes(t *testing.T) {
	svc, sched, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	rev1 := createEntryWithRevision(t, queries, "e4", "post", "e4", "public", 0, "draft")
	now := int64(1000)
	svc.PublishRevision(ctx, "e4", rev1, now)
	rev2ID := "e4-r2"
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2ID, EntryID: "e4", RevisionNumber: 2, Slug: "e4", Title: "R2", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 5})
	svc.Schedule(ctx, "e4", rev2ID, 1500, "", now+5)
	// Simulate overdue: now is 5000, job scheduled at 1500
	sched.RunDue(ctx, 5000)
	entry, _ := queries.GetEntry(ctx, "e4")
	if entry.PublishedRevisionID.String != rev2ID {
		t.Fatalf("overdue should publish")
	}
}

func TestSchedulerCancelNoPublish(t *testing.T) {
	svc, sched, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	rev1 := createEntryWithRevision(t, queries, "e5", "post", "e5", "public", 0, "draft")
	now := int64(1000)
	svc.PublishRevision(ctx, "e5", rev1, now)
	rev2ID := "e5-r2"
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2ID, EntryID: "e5", RevisionNumber: 2, Slug: "e5", Title: "R2", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 5})
	svc.Schedule(ctx, "e5", rev2ID, 2000, "", now+5)
	svc.CancelSchedule(ctx, "e5", now+6)
	sched.RunDue(ctx, 3000)
	entry, _ := queries.GetEntry(ctx, "e5")
	if entry.PublishedRevisionID.String == rev2ID {
		t.Fatalf("cancelled should not publish")
	}
}

func TestSchedulerTrashedEntryFails(t *testing.T) {
	svc, sched, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	rev1 := createEntryWithRevision(t, queries, "e6", "post", "e6", "public", 0, "draft")
	now := int64(1000)
	svc.PublishRevision(ctx, "e6", rev1, now)
	rev2ID := "e6-r2"
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2ID, EntryID: "e6", RevisionNumber: 2, Slug: "e6", Title: "R2", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 5})
	svc.Schedule(ctx, "e6", rev2ID, 2000, "", now+5)
	// Trash entry
	queries.MoveEntryToTrash(ctx, db.MoveEntryToTrashParams{ID: "e6", TrashedAt: sql.NullInt64{Int64: now + 10, Valid: true}, UpdatedAt: now + 10})
	// Scheduler should attempt and mark failed
	sched.RunDue(ctx, 3000)
	jobRows, _ := queries.ListPublicationJobsForEntry(ctx, "e6")
	foundFailed := false
	for _, j := range jobRows {
		if j.Status == "failed" {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Fatalf("trashed entry job should be failed")
	}
	entry, _ := queries.GetEntry(ctx, "e6")
	if entry.PublishedRevisionID.String == rev2ID {
		t.Fatalf("trashed entry should not publish")
	}
}

func TestSchedulerMissingRevisionFails(t *testing.T) {
	_, sched, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	now := int64(1000)
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "e7", ContentTypeID: "post", Slug: "e7", Status: "active", CreatedAt: now, UpdatedAt: now})
	// Create a valid revision then schedule, then delete revision to simulate missing
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "e7-r1", EntryID: "e7", RevisionNumber: 1, Slug: "e7", Title: "R1", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now})
	queries.CreatePublicationJob(ctx, db.CreatePublicationJobParams{ID: "job-missing", EntryID: "e7", RevisionID: "e7-r1", ScheduledAt: 1500, CreatedAt: now, UpdatedAt: now})
	// Delete revision with FK off to simulate missing
	_, _ = database.DB.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
	_, _ = database.DB.ExecContext(ctx, "DELETE FROM entry_revisions WHERE id = ?", "e7-r1")
	_, _ = database.DB.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	sched.RunDue(ctx, 2000)
	job, _ := queries.GetPublicationJob(ctx, "job-missing")
	if job.Status != "failed" {
		t.Fatalf("missing revision should be failed, got %s", job.Status)
	}
}

func TestSchedulerRevisionMismatchFails(t *testing.T) {
	_, sched, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	now := int64(1000)
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "e8", ContentTypeID: "post", Slug: "e8", Status: "active", CreatedAt: now, UpdatedAt: now})
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "e9", ContentTypeID: "post", Slug: "e9", Status: "active", CreatedAt: now, UpdatedAt: now})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "e9-r1", EntryID: "e9", RevisionNumber: 1, Slug: "e9", Title: "R1", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now})
	queries.CreatePublicationJob(ctx, db.CreatePublicationJobParams{ID: "job-mismatch", EntryID: "e8", RevisionID: "e9-r1", ScheduledAt: 1500, CreatedAt: now, UpdatedAt: now})
	sched.RunDue(ctx, 2000)
	job, _ := queries.GetPublicationJob(ctx, "job-mismatch")
	if job.Status != "failed" {
		t.Fatalf("mismatch should be failed, got %s", job.Status)
	}
}

func TestSchedulerHierarchyInvalidFails(t *testing.T) {
	svc, sched, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	// Create parent and child, publish child, then schedule parent private which should fail at publish time (hierarchy check)
	parentRev := createEntryWithRevision(t, queries, "hp", "page", "hp", "public", 0, "draft")
	_ = createEntryWithRevision(t, queries, "hc", "page", "hc", "public", 0, "draft")
	now := int64(1000)
	// child with parent hp
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "hc-r2", EntryID: "hc", RevisionNumber: 2, Slug: "hc", Title: "HC", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 5, ParentEntryID: sql.NullString{String: "hp", Valid: true}})
	svc.PublishRevision(ctx, "hp", parentRev, now)
	svc.PublishRevision(ctx, "hc", "hc-r2", now)
	// Now schedule parent as private (should fail when scheduler tries to publish)
	now2 := now + 10
	privID := "hp-r2"
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: privID, EntryID: "hp", RevisionNumber: 2, Slug: "hp", Title: "HP private", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now2, Visibility: "private"})
	svc.Schedule(ctx, "hp", privID, 2000, "", now2)
	sched.RunDue(ctx, 3000)
	jobRows, _ := queries.ListPublicationJobsForEntry(ctx, "hp")
	foundFailed := false
	for _, j := range jobRows {
		if j.ScheduledAt == 2000 && j.Status == "failed" {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Fatalf("private parent with child should fail")
	}
}

func TestSchedulerPrivateParentRejected(t *testing.T) {
	// Similar to above but already tested; ensure it fails
	TestSchedulerHierarchyInvalidFails(t)
}

func TestSchedulerNewScheduleReplacesPrevious(t *testing.T) {
	svc, _, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	rev1 := createEntryWithRevision(t, queries, "e10", "post", "e10", "public", 0, "draft")
	now := int64(1000)
	svc.PublishRevision(ctx, "e10", rev1, now)
	rev2ID := "e10-r2"
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2ID, EntryID: "e10", RevisionNumber: 2, Slug: "e10", Title: "R2", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 5})
	rev3ID := "e10-r3"
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev3ID, EntryID: "e10", RevisionNumber: 3, Slug: "e10", Title: "R3", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 6})
	svc.Schedule(ctx, "e10", rev2ID, 2000, "", now+5)
	svc.Schedule(ctx, "e10", rev3ID, 3000, "", now+6)
	jobs, _ := queries.ListPublicationJobsForEntry(ctx, "e10")
	active := 0
	for _, j := range jobs {
		if j.Status == "scheduled" {
			active++
			if j.RevisionID != rev3ID {
				t.Fatalf("active should be R3, got %s", j.RevisionID)
			}
		}
	}
	if active != 1 {
		t.Fatalf("should have exactly one active, got %d", active)
	}
}
