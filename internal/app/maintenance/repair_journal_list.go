package maintenance

import "context"

// ListRepairs answers "what did the repairs touch" across the library: every
// journal entry in the window, newest first, optionally one batch kind. The
// only way the 98 files of 2026-07-13 were ever found was a hand-written SQL
// query on file modification times; this is that question, asked of the
// journal, from the health page.
//
// A journal that cannot list (nil, or a per-file-only implementation) returns
// nothing rather than an error: the page then shows an empty journal, which is
// what it is.
func (s *SeekIndexService) ListRepairs(ctx context.Context, q RepairQuery) ([]RepairRecord, error) {
	lister, ok := s.journal.(RepairLister)
	if !ok {
		return nil, nil
	}
	if q.Limit <= 0 {
		q.Limit = 100
	}
	return lister.ListRepairs(ctx, q)
}
