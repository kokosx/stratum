package publishing

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/routing"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"golang.org/x/crypto/bcrypt"
)

// Service owns the shared publication semantics for both admin immediate publish and scheduler.
type Service struct {
	db            *sql.DB
	queries       *db.Queries
	searchRefresh func(context.Context, string) error
}

func New(database *sql.DB, queries *db.Queries) *Service {
	return &Service{db: database, queries: queries}
}

// SetSearchRefresh wires the rebuildable search projection without making
// publication depend on a concrete search package.
func (s *Service) SetSearchRefresh(fn func(context.Context, string) error) { s.searchRefresh = fn }

// PublishRevision atomically publishes the exact revision. It is the single implementation used by admin and scheduler.
func (s *Service) PublishRevision(ctx context.Context, entryID, revisionID string, now int64) error {
	if s.db == nil {
		return errors.New("database is not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin publish: %w", err)
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	entry, err := qtx.GetEntry(ctx, entryID)
	if err != nil {
		return fmt.Errorf("entry not found: %w", err)
	}
	if entry.Status == "trash" {
		return errors.New("cannot publish trashed entry")
	}
	rev, err := qtx.GetEntryRevision(ctx, revisionID)
	if err != nil {
		return fmt.Errorf("revision not found: %w", err)
	}
	if rev.EntryID != entryID {
		return errors.New("revision does not belong to entry")
	}
	if err := PublishWithQueries(ctx, qtx, entry, rev, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit publish: %w", err)
	}
	if s.searchRefresh != nil {
		if err := s.searchRefresh(context.Background(), entryID); err != nil {
			log.Printf("search refresh after publishing entry %s: %v (publication remains committed)", entryID, err)
		}
	}
	return nil
}

func validateVisibility(visibility string, hash sql.NullString) error {
	switch visibility {
	case "public":
		if hash.Valid && hash.String != "" {
			return errors.New("public visibility cannot have password hash")
		}
	case "private":
		if hash.Valid && hash.String != "" {
			return errors.New("private visibility cannot have password hash")
		}
	case "password":
		if !hash.Valid || strings.TrimSpace(hash.String) == "" {
			return errors.New("password protected visibility requires password hash")
		}
		if _, err := bcrypt.Cost([]byte(hash.String)); err != nil {
			return errors.New("invalid password hash")
		}
	default:
		return fmt.Errorf("invalid visibility %q", visibility)
	}
	return nil
}

func validatePostsBlocksForPublish(ctx context.Context, qtx *db.Queries, entryID string, doc *document.Document) error {
	settings, err := qtx.GetSiteSettings(ctx)
	if err != nil {
		return nil
	}
	if !settings.PostsPageEntryID.Valid || settings.PostsPageEntryID.String != entryID {
		return nil
	}
	if doc == nil {
		return nil
	}
	count := 0
	var walk func([]document.Node)
	walk = func(nodes []document.Node) {
		for _, n := range nodes {
			if n.Block == "core/posts" {
				source := "archive"
				pagination := true
				if len(n.Settings) > 0 {
					var s map[string]any
					if json.Unmarshal(n.Settings, &s) == nil {
						if v, ok := s["source"].(string); ok && v != "" {
							source = v
						}
						if v, ok := s["pagination"].(bool); ok {
							pagination = v
						}
					}
				}
				if source == "archive" && pagination {
					count++
				}
			}
			if len(n.Children) > 0 {
				walk(n.Children)
			}
		}
	}
	walk(doc.Nodes)
	if count > 1 {
		return errors.New("Only one paginated archive Posts block can be used on a Posts Page.")
	}
	return nil
}

func nullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// PublishWithQueries is the shared publication transaction helper used by both admin immediate publish and scheduler.
// It validates the revision and updates routes/published_revision within the provided transaction.
func PublishWithQueries(ctx context.Context, qtx *db.Queries, entry db.Entry, rev db.EntryRevision, now int64) error {
	if err := validateVisibility(rev.Visibility, rev.PasswordHash); err != nil {
		return err
	}
	if rev.ReviewState != "draft" && rev.ReviewState != "pending" {
		return fmt.Errorf("invalid review state %q", rev.ReviewState)
	}
	doc, err := document.Decode([]byte(rev.DocumentJson))
	if err != nil {
		return fmt.Errorf("invalid document: %w", err)
	}
	settings, err := qtx.GetSiteSettings(ctx)
	if err != nil {
		return fmt.Errorf("load site settings: %w", err)
	}
	// DB-backed definitions are required for custom routing and capabilities;
	// the builtin fallback retains compatibility with pre-catalog installations.
	def, defErr := content.NewCatalog(qtx).GetDefinition(ctx, entry.ContentTypeID)
	if defErr != nil {
		if entry.ContentTypeID == string(content.ContentTypePage) || entry.ContentTypeID == string(content.ContentTypePost) {
			def = content.DefinitionFor(entry.ContentTypeID)
		} else {
			return fmt.Errorf("load content type %s: %w", entry.ContentTypeID, defErr)
		}
	}
	isPostsPage := settings.PostsPageEntryID.Valid && settings.PostsPageEntryID.String == entry.ID
	isHomepage := settings.HomepageEntryID.Valid && settings.HomepageEntryID.String == entry.ID
	// Homepage and Posts Page must remain publicly routable; they cannot be private or password-protected.
	if (isHomepage || isPostsPage) && (rev.Visibility == "private" || rev.Visibility == "password") {
		return content.ErrProtectedPage
	}
	// Non-public content types never expose public canonical routes or archives.
	// They can still be marked as published internally, but the frontend must not routable.
	if !def.Capabilities.Public {
		if route, err := qtx.GetEntryRoute(ctx, sql.NullString{String: entry.ID, Valid: true}); err == nil {
			if err := qtx.DeleteRoute(ctx, route.ID); err != nil {
				return fmt.Errorf("remove non-public route: %w", err)
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check non-public route: %w", err)
		}
	} else {
		// Publishing a hierarchical entry as private removes its route; reject if public descendants would become orphaned.
		if rev.Visibility == "private" && def.Capabilities.Hierarchical {
			rows, err := qtx.ListPublishedHierarchyForContentType(ctx, entry.ContentTypeID)
			if err != nil {
				return err
			}
			nodes := make([]content.HierarchyNode, 0, len(rows))
			for _, r := range rows {
				parent := ""
				if r.ParentEntryID.Valid {
					parent = r.ParentEntryID.String
				}
				nodes = append(nodes, content.HierarchyNode{EntryID: r.EntryID, ParentEntryID: parent})
			}
			h, err := content.NewHierarchy(nodes)
			if err != nil {
				return err
			}
			if len(h.Descendants(entry.ID)) != 0 {
				return content.ErrPublishedDescendants
			}
		}
		switch rev.Visibility {
		case "private":
			if route, err := qtx.GetEntryRoute(ctx, sql.NullString{String: entry.ID, Valid: true}); err == nil {
				if err := qtx.DeleteRoute(ctx, route.ID); err != nil {
					return fmt.Errorf("remove private route: %w", err)
				}
			} else if !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("check private route: %w", err)
			}
		case "public", "password":
			if isPostsPage && entry.ContentTypeID == "page" {
				oldBase := settings.PostsBasePath
				if oldBase == "" {
					oldBase = routing.DefaultPostsBase
				}
				newBase := routing.NormalizePath("/" + strings.Trim(rev.Slug, "/"))
				if oldBase != newBase {
					if err := routing.ValidatePostsBasePath(newBase); err != nil {
						return err
					}
					if err := routing.SyncPostsPageSlugChanged(ctx, qtx, entry.ID, rev.Slug, oldBase, newBase, settings.HomepageMode, now); err != nil {
						return err
					}
				}
			} else if def.Capabilities.Hierarchical {
				if _, err := routing.SyncHierarchyPublish(ctx, qtx, def, routing.HierarchyEntry{
					EntryID: entry.ID, ContentTypeID: entry.ContentTypeID, Slug: rev.Slug, Status: "active", Title: rev.Title,
					ParentEntryID: nullString(rev.ParentEntryID), MenuOrder: rev.MenuOrder,
				}, now); err != nil {
					return err
				}
			} else {
				computedPath := routing.EntryPathForDefinition(def, rev.Slug, settings.PostsBasePath)
				if err := routing.UpsertEntryRoute(ctx, qtx, entry.ID, computedPath, now); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unknown visibility %q", rev.Visibility)
		}
	}
	if isPostsPage && rev.Visibility != "private" {
		if err := validatePostsBlocksForPublish(ctx, qtx, entry.ID, doc); err != nil {
			return err
		}
	}
	if err := qtx.SetFirstPublishedAtIfNull(ctx, db.SetFirstPublishedAtIfNullParams{FirstPublishedAt: sql.NullInt64{Int64: now, Valid: true}, ID: entry.ID}); err != nil {
		return fmt.Errorf("record first publication: %w", err)
	}
	if err := qtx.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: rev.ID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: entry.ID}); err != nil {
		return fmt.Errorf("publish entry revision: %w", err)
	}
	return nil
}

// HashPassword hashes a plaintext password for revision storage.
func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword verifies plain against hash.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// Schedule creates a durable job to publish revisionID at scheduledAt. It atomically cancels any existing active schedule.
func (s *Service) Schedule(ctx context.Context, entryID, revisionID string, scheduledAt int64, createdBy string, now int64) error {
	if scheduledAt <= now {
		return errors.New("scheduled time must be in the future")
	}
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		return err
	}
	if entry.Status == "trash" {
		return errors.New("cannot schedule trashed entry")
	}
	rev, err := s.queries.GetEntryRevision(ctx, revisionID)
	if err != nil {
		return err
	}
	if rev.EntryID != entryID {
		return errors.New("revision does not belong to entry")
	}
	if err := validateVisibility(rev.Visibility, rev.PasswordHash); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	// Cancel previous scheduled jobs for entry
	if err := qtx.CancelActivePublicationJobsForEntry(ctx, db.CancelActivePublicationJobsForEntryParams{
		UpdatedAt: now, LastError: sql.NullString{}, EntryID: entryID,
	}); err != nil {
		return err
	}
	// Create new job
	id, err := randomID()
	if err != nil {
		return err
	}
	var createdByNS sql.NullString
	if strings.TrimSpace(createdBy) != "" {
		createdByNS = sql.NullString{String: createdBy, Valid: true}
	}
	if err := qtx.CreatePublicationJob(ctx, db.CreatePublicationJobParams{
		ID: id, EntryID: entryID, RevisionID: revisionID, ScheduledAt: scheduledAt, CreatedBy: createdByNS, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// CancelSchedule cancels any active scheduled job for entry.
func (s *Service) CancelSchedule(ctx context.Context, entryID string, now int64) error {
	return s.queries.CancelActivePublicationJobsForEntry(ctx, db.CancelActivePublicationJobsForEntryParams{
		UpdatedAt: now, EntryID: entryID, LastError: sql.NullString{String: "cancelled by user", Valid: true},
	})
}

// Unpublish clears published revision and cancels any scheduled job.
// Validation happens before any destructive mutation.
func (s *Service) Unpublish(ctx context.Context, entryID string, now int64) error {
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		return err
	}
	if s.db == nil {
		return errors.New("database not configured")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	// Validate special-page protection before mutation.
	settings, err := qtx.GetSiteSettings(ctx)
	if err != nil {
		return err
	}
	if settings.HomepageEntryID.Valid && settings.HomepageEntryID.String == entry.ID {
		return content.ErrProtectedPage
	}
	if settings.PostsPageEntryID.Valid && settings.PostsPageEntryID.String == entry.ID {
		return content.ErrProtectedPage
	}

	// Validate hierarchical descendants before destructive changes.
	if content.DefinitionFor(entry.ContentTypeID).Capabilities.Hierarchical {
		rows, err := qtx.ListPublishedHierarchyForContentType(ctx, entry.ContentTypeID)
		if err != nil {
			return err
		}
		nodes := make([]content.HierarchyNode, 0, len(rows))
		for _, r := range rows {
			parent := ""
			if r.ParentEntryID.Valid {
				parent = r.ParentEntryID.String
			}
			nodes = append(nodes, content.HierarchyNode{EntryID: r.EntryID, ParentEntryID: parent})
		}
		h, err := content.NewHierarchy(nodes)
		if err != nil {
			return err
		}
		if len(h.Descendants(entryID)) != 0 {
			return content.ErrPublishedDescendants
		}
	}

	// Cancel active scheduled publication atomically within transaction.
	if err := qtx.CancelActivePublicationJobsForEntry(ctx, db.CancelActivePublicationJobsForEntryParams{
		UpdatedAt: now, EntryID: entryID, LastError: sql.NullString{String: "cancelled by unpublish", Valid: true},
	}); err != nil {
		return err
	}
	if err := qtx.ClearPublishedRevision(ctx, db.ClearPublishedRevisionParams{UpdatedAt: now, ID: entryID}); err != nil {
		return err
	}
	if route, err := qtx.GetEntryRoute(ctx, sql.NullString{String: entryID, Valid: true}); err == nil {
		if err := qtx.DeleteRoute(ctx, route.ID); err != nil {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.searchRefresh != nil {
		if err := s.searchRefresh(context.Background(), entryID); err != nil {
			log.Printf("search removal after unpublishing entry %s: %v (unpublish remains committed)", entryID, err)
		}
	}
	return nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
