package media

import (
	"context"
	"fmt"
)

// DeleteIfUnused removes an asset only if it has no known structured references.
// It returns ErrInUse when the asset is still referenced.
func (s *Service) DeleteIfUnused(ctx context.Context, id string) error {
	refs, err := s.UsageRefs(ctx, id)
	if err != nil {
		return err
	}
	if len(refs) > 0 {
		return fmt.Errorf("%w: used in %d place(s)", ErrInUse, len(refs))
	}
	return s.Delete(ctx, id)
}
