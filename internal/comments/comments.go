package comments

import (
	"errors"
	"html"
	"strings"
)

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusSpam     = "spam"
	StatusTrash    = "trash"

	MaxNameLength  = 100
	MaxEmailLength = 254
	MaxURLLength   = 2048
	MaxBodyLength  = 5000
	MinBodyLength  = 1
	MaxDepth       = 3
	MaxPerEntry    = 100
)

var (
	ErrInvalidName      = errors.New("name is required")
	ErrInvalidEmail     = errors.New("invalid email address")
	ErrInvalidURL       = errors.New("invalid URL")
	ErrBodyRequired     = errors.New("comment body is required")
	ErrBodyTooLong      = errors.New("comment is too long")
	ErrParentInvalid    = errors.New("invalid parent comment")
	ErrDepthExceeded    = errors.New("reply depth limit exceeded")
	ErrCommentsDisabled = errors.New("comments are closed for this entry")
	ErrNotCommentable   = errors.New("entry does not support comments")
	ErrRateLimited      = errors.New("you are posting too quickly")
	ErrHoneypot         = errors.New("spam detected")
	ErrNotFound         = errors.New("comment not found")
	ErrInvalidStatus    = errors.New("invalid status")
)

// CommentView is public projection (email never included)
type CommentView struct {
	ID              string
	EntryID         string
	ParentID        string
	AuthorName      string
	Body            string // escaped plain text with line breaks
	CreatedAt       int64
	CreatedISO      string
	CreatedAtString string
	Depth           int
}

// HTML helpers
func escapeBody(body string) string {
	escaped := html.EscapeString(body)
	// preserve line breaks as <br> is handled in template via white-space:pre-wrap, but we also replace \n with <br> for safety
	// Keep as plain text, template will escape again? We pre-escape and then render as HTML with line breaks preserved via CSS.
	return escaped
}

func normalizeBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.TrimSpace(body)
	return body
}
