package comments

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/kokosx/stratum/internal/content"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

type Service struct {
	db         *sql.DB
	queries    *db.Queries
	limiter    *rateLimiter
	invalidate func(entryID string)
}

func NewService(database *sql.DB, queries *db.Queries) *Service {
	return &Service{
		db:      database,
		queries: queries,
		limiter: newRateLimiter(),
	}
}

func (s *Service) SetInvalidator(fn func(entryID string)) {
	s.invalidate = fn
}

// Submit handles public comment submission with all eligibility checks.
// hasUnlock indicates whether the request has a valid password unlock token for the entry.
func (s *Service) Submit(ctx context.Context, entryID, parentID, authorName, authorEmail, authorURL, body, honeypot, userID, role, ip string, hasUnlock bool, now int64) (db.Comment, error) {
	if strings.TrimSpace(honeypot) != "" {
		return db.Comment{}, ErrHoneypot
	}
	// Rate limit (anonymous only, but apply to all for simplicity)
	key := clientIPKey(ip, entryID)
	if !s.limiter.Allow(key, now) {
		return db.Comment{}, ErrRateLimited
	}

	// Load entry and published revision
	entry, err := s.queries.GetEntry(ctx, entryID)
	if err != nil {
		return db.Comment{}, ErrNotCommentable
	}
	if entry.Status == "trash" {
		return db.Comment{}, ErrNotCommentable
	}
	if !entry.PublishedRevisionID.Valid {
		return db.Comment{}, ErrNotCommentable
	}
	rev, err := s.queries.GetEntryRevision(ctx, entry.PublishedRevisionID.String)
	if err != nil {
		return db.Comment{}, ErrNotCommentable
	}
	// Check content type supports comments
	def := content.DefinitionFor(entry.ContentTypeID)
	if !def.Capabilities.SupportsComments {
		return db.Comment{}, ErrNotCommentable
	}
	if rev.CommentsEnabled == 0 {
		return db.Comment{}, ErrCommentsDisabled
	}
	// Check visibility / password
	switch rev.Visibility {
	case "private":
		return db.Comment{}, ErrNotCommentable
	case "password":
		if !hasUnlock {
			return db.Comment{}, ErrNotCommentable
		}
	case "public":
		// ok
	default:
		return db.Comment{}, ErrNotCommentable
	}

	// Validate parent
	var parentNull sql.NullString
	if strings.TrimSpace(parentID) != "" {
		parentID = strings.TrimSpace(parentID)
		p, err := s.queries.GetComment(ctx, parentID)
		if err != nil {
			return db.Comment{}, ErrParentInvalid
		}
		if p.EntryID != entryID {
			return db.Comment{}, ErrParentInvalid
		}
		if p.Status != StatusApproved && p.Status != StatusPending {
			return db.Comment{}, ErrParentInvalid
		}
		// depth check
		depth, err := s.depthFor(ctx, parentID)
		if err != nil {
			return db.Comment{}, err
		}
		// MaxDepth counts visible levels: top-level is 1, so a parent at
		// depth 2 may still receive the final (third-level) reply.
		if depth+1 > MaxDepth {
			return db.Comment{}, ErrDepthExceeded
		}
		// cycle check: parent cannot be descendant of new comment (not possible since new comment not yet exists) but check that parent chain doesn't contain cycle
		if err := s.checkCycle(ctx, parentID); err != nil {
			return db.Comment{}, err
		}
		parentNull = sql.NullString{String: parentID, Valid: true}
	}

	// Validate fields
	isLoggedIn := strings.TrimSpace(userID) != ""
	if isLoggedIn {
		u, err := s.queries.GetUserByID(ctx, userID)
		if err != nil || strings.TrimSpace(u.Email) == "" {
			return db.Comment{}, ErrNotCommentable
		}
		// Form fields are never an identity authority for authenticated users.
		authorEmail = u.Email
		authorName = strings.TrimSpace(strings.Split(u.Email, "@")[0])
		if authorName == "" {
			authorName = "User"
		}
	} else {
		if err := validateName(authorName); err != nil {
			return db.Comment{}, err
		}
		if err := validateEmail(authorEmail); err != nil {
			return db.Comment{}, err
		}
	}
	if err := validateURL(authorURL); err != nil {
		return db.Comment{}, err
	}
	body = normalizeBody(body)
	if err := validateBody(body); err != nil {
		return db.Comment{}, err
	}

	// Determine initial status
	status := StatusPending
	if isLoggedIn && (role == "admin" || role == "editor") {
		status = StatusApproved
	}

	id, err := randomID()
	if err != nil {
		return db.Comment{}, err
	}
	var userIDNull sql.NullString
	if isLoggedIn {
		userIDNull = sql.NullString{String: userID, Valid: true}
	}
	params := db.CreateCommentParams{
		ID:                 id,
		EntryID:            entryID,
		ParentID:           parentNull,
		Status:             status,
		AuthorName:         strings.TrimSpace(authorName),
		AuthorEmail:        strings.TrimSpace(authorEmail),
		AuthorUrl:          strings.TrimSpace(authorURL),
		UserID:             userIDNull,
		Body:               body,
		CreatedAt:          now,
		UpdatedAt:          now,
		ImportedSource:     sql.NullString{},
		ImportedExternalID: sql.NullString{},
	}
	if err := s.queries.CreateComment(ctx, params); err != nil {
		return db.Comment{}, err
	}
	s.limiter.Record(key, now)
	// Invalidate cache only if approved (public visible)
	if status == StatusApproved && s.invalidate != nil {
		s.invalidate(entryID)
	}
	c, _ := s.queries.GetComment(ctx, id)
	return c, nil
}

func (s *Service) depthFor(ctx context.Context, parentID string) (int, error) {
	depth := 0
	cur := parentID
	seen := map[string]bool{}
	for cur != "" {
		if seen[cur] {
			return 0, errors.New("cycle detected")
		}
		seen[cur] = true
		c, err := s.queries.GetComment(ctx, cur)
		if err != nil {
			return depth, nil
		}
		depth++
		if depth > MaxDepth {
			return depth, nil
		}
		if !c.ParentID.Valid {
			break
		}
		cur = c.ParentID.String
		if len(seen) > MaxDepth+5 {
			break
		}
	}
	return depth, nil
}

func (s *Service) checkCycle(ctx context.Context, parentID string) error {
	seen := map[string]bool{parentID: true}
	cur := parentID
	for cur != "" {
		c, err := s.queries.GetComment(ctx, cur)
		if err != nil {
			break
		}
		if !c.ParentID.Valid {
			break
		}
		parent := c.ParentID.String
		if seen[parent] {
			return errors.New("cycle detected")
		}
		seen[parent] = true
		cur = parent
		if len(seen) > 10 {
			break
		}
	}
	return nil
}

// ListApproved returns approved comments for public rendering, oldest first, limited.
func (s *Service) ListApproved(ctx context.Context, entryID string) ([]db.Comment, error) {
	return s.queries.ListApprovedCommentsByEntry(ctx, db.ListApprovedCommentsByEntryParams{EntryID: entryID, Limit: MaxPerEntry, Offset: 0})
}

func (s *Service) CountApproved(ctx context.Context, entryID string) (int64, error) {
	return s.queries.CountApprovedCommentsByEntry(ctx, entryID)
}

// Moderate methods

func (s *Service) Approve(ctx context.Context, id string, now int64) error {
	c, err := s.queries.GetComment(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if c.Status == StatusApproved {
		return nil
	}
	if err := s.queries.UpdateCommentStatus(ctx, db.UpdateCommentStatusParams{Status: StatusApproved, UpdatedAt: now, ID: id}); err != nil {
		return err
	}
	if s.invalidate != nil {
		s.invalidate(c.EntryID)
	}
	return nil
}

func (s *Service) SetPending(ctx context.Context, id string, now int64) error {
	c, err := s.queries.GetComment(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	prev := c.Status
	if err := s.queries.UpdateCommentStatus(ctx, db.UpdateCommentStatusParams{Status: StatusPending, UpdatedAt: now, ID: id}); err != nil {
		return err
	}
	if prev == StatusApproved && s.invalidate != nil {
		s.invalidate(c.EntryID)
	}
	return nil
}

func (s *Service) Spam(ctx context.Context, id string, now int64) error {
	c, err := s.queries.GetComment(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	prev := c.Status
	if err := s.queries.UpdateCommentStatus(ctx, db.UpdateCommentStatusParams{Status: StatusSpam, UpdatedAt: now, ID: id}); err != nil {
		return err
	}
	if prev == StatusApproved && s.invalidate != nil {
		s.invalidate(c.EntryID)
	}
	return nil
}

func (s *Service) Trash(ctx context.Context, id string, now int64) error {
	c, err := s.queries.GetComment(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	prev := c.Status
	if err := s.queries.UpdateCommentStatus(ctx, db.UpdateCommentStatusParams{Status: StatusTrash, UpdatedAt: now, ID: id}); err != nil {
		return err
	}
	if prev == StatusApproved && s.invalidate != nil {
		s.invalidate(c.EntryID)
	}
	return nil
}

func (s *Service) Restore(ctx context.Context, id string, now int64) error {
	c, err := s.queries.GetComment(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if c.Status != StatusTrash {
		return ErrInvalidStatus
	}
	// Restore to pending
	if err := s.queries.UpdateCommentStatus(ctx, db.UpdateCommentStatusParams{Status: StatusPending, UpdatedAt: now, ID: id}); err != nil {
		return err
	}
	// Pending does not invalidate (not public)
	return nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	c, err := s.queries.GetComment(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	wasApproved := c.Status == StatusApproved
	if err := s.queries.DeleteComment(ctx, id); err != nil {
		return err
	}
	if wasApproved && s.invalidate != nil {
		s.invalidate(c.EntryID)
	}
	return nil
}

// Bulk operations

func (s *Service) BulkUpdateStatus(ctx context.Context, ids []string, status string, now int64) error {
	var errs []error
	for _, id := range ids {
		var err error
		switch status {
		case StatusApproved:
			err = s.Approve(ctx, id, now)
		case StatusPending:
			err = s.SetPending(ctx, id, now)
		case StatusSpam:
			err = s.Spam(ctx, id, now)
		case StatusTrash:
			err = s.Trash(ctx, id, now)
		default:
			return ErrInvalidStatus
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ListFiltered for admin
func (s *Service) ListFiltered(ctx context.Context, status, search, entryID string, limit, offset int) ([]db.Comment, int64, error) {
	var statusArg interface{}
	if status != "" && status != "all" {
		statusArg = status
	}
	var searchArg interface{}
	if strings.TrimSpace(search) != "" {
		searchArg = strings.TrimSpace(search)
	}
	var entryArg interface{}
	if strings.TrimSpace(entryID) != "" {
		entryArg = strings.TrimSpace(entryID)
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.queries.ListCommentsFiltered(ctx, db.ListCommentsFilteredParams{Status: statusArg, Search: searchArg, EntryID: entryArg, Limit: int64(limit), Offset: int64(offset)})
	if err != nil {
		return nil, 0, err
	}
	total, err := s.queries.CountCommentsFiltered(ctx, db.CountCommentsFilteredParams{Status: statusArg, Search: searchArg, EntryID: entryArg})
	if err != nil {
		return rows, 0, err
	}
	return rows, total, nil
}

func (s *Service) CountByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := s.queries.CountCommentsByStatus(ctx)
	if err != nil {
		return nil, err
	}
	m := map[string]int64{StatusPending: 0, StatusApproved: 0, StatusSpam: 0, StatusTrash: 0}
	for _, r := range rows {
		m[r.Status] = r.Count
	}
	return m, nil
}

// Import helpers

func (s *Service) CreateImported(ctx context.Context, entryID, parentID, authorName, authorEmail, authorURL, body, importedSource, importedExternalID string, status string, createdAt int64) (db.Comment, error) {
	// Validate body length, etc., but skip rate limit/honeypot
	body = normalizeBody(body)
	if body == "" {
		body = "(empty)"
	}
	if len(body) > MaxBodyLength {
		body = body[:MaxBodyLength]
	}
	var parentNull sql.NullString
	if strings.TrimSpace(parentID) != "" {
		parentNull = sql.NullString{String: strings.TrimSpace(parentID), Valid: true}
	}
	id, err := randomID()
	if err != nil {
		return db.Comment{}, err
	}
	params := db.CreateCommentParams{
		ID:                 id,
		EntryID:            entryID,
		ParentID:           parentNull,
		Status:             status,
		AuthorName:         authorName,
		AuthorEmail:        authorEmail,
		AuthorUrl:          authorURL,
		UserID:             sql.NullString{},
		Body:               body,
		CreatedAt:          createdAt,
		UpdatedAt:          createdAt,
		ImportedSource:     sql.NullString{String: importedSource, Valid: importedSource != ""},
		ImportedExternalID: sql.NullString{String: importedExternalID, Valid: importedExternalID != ""},
	}
	if err := s.queries.CreateComment(ctx, params); err != nil {
		return db.Comment{}, err
	}
	if status == StatusApproved && s.invalidate != nil {
		s.invalidate(entryID)
	}
	return s.queries.GetComment(ctx, id)
}
