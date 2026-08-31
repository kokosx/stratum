package entryops

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrValidation       = errors.New("validation failed")
	ErrForbidden        = errors.New("forbidden")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrConflict         = errors.New("revision conflict")
	ErrInvalidContentType = errors.New("invalid content type")
	ErrProtectedPage    = errors.New("protected page")
	ErrTrashProtected   = errors.New("cannot trash protected page")
)

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string { return fmt.Sprintf("%s: %s", e.Field, e.Message) }

type ConflictError struct {
	Expected string
	Current  string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("revision conflict: expected %s but current is %s", e.Expected, e.Current)
}

func IsConflict(err error) bool {
	var ce *ConflictError
	return errors.As(err, &ce)
}

type ForbiddenError struct {
	Permission string
	Scope      string
}

func (e *ForbiddenError) Error() string {
	if e.Scope != "" {
		return fmt.Sprintf("forbidden: missing %s (%s)", e.Permission, e.Scope)
	}
	return fmt.Sprintf("forbidden: missing %s", e.Permission)
}

func isValidationError(err error) bool {
	return errors.Is(err, ErrValidation)
}
