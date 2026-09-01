package maintenance

import (
	"context"
	"errors"
	"time"
)

// ErrAckNotApplicable is returned when the operator tries to acknowledge a file
// whose verdict is repairable (or healthy): the page repairs what it can, it
// does not let a fixable defect be waved through and forgotten.
var ErrAckNotApplicable = errors.New("seek-index: only a file nothing can repair here can be acknowledged")

// ErrNotInScan is returned when a media is not part of any cached scan - the
// operator acknowledges what the page shows, and the page shows scans.
var ErrNotInScan = errors.New("seek-index: media not in any completed scan")

// ackNow is swapped in tests.
var ackNow = func() int64 { return time.Now().Unix() }

// AckMedia records (or withdraws) the operator's acknowledgement of one file's
// verdict, across every cached scope that holds it, and persists it. Refused
// for a repairable file - see ErrAckNotApplicable.
func (s *SeekIndexService) AckMedia(_ context.Context, mediaID string, acked bool) error {
	s.mu.Lock()
	var found, ackable bool
	for _, c := range s.scans {
		for i := range c.res.Items {
			if c.res.Items[i].MediaID == mediaID {
				found = true
				ackable = c.res.Items[i].Ackable()
			}
		}
	}
	s.mu.Unlock()
	if !found {
		return ErrNotInScan
	}
	if acked && !ackable {
		return ErrAckNotApplicable
	}
	stamp := int64(0)
	if acked {
		stamp = ackNow()
	}
	s.updateCachedMedia(mediaID, func(st *SeekStatus) { st.AckedAt = stamp })
	s.saveScans()
	return nil
}

// AckGroup acknowledges every not-yet-acknowledged file of the given verdict
// in scope (libraryID "" = all libraries), mirroring the stamp into every
// other scope holding the same media. Returns how many files it marked. Only
// the informational verdict is a group one can acknowledge - a repairable
// defect is repaired, not waved through.
func (s *SeekIndexService) AckGroup(_ context.Context, libraryID string, verdict Verdict) (int, error) {
	if verdict != VerdictInformational {
		return 0, ErrAckNotApplicable
	}
	res, _, ok := s.CachedScan(libraryID)
	if !ok {
		return 0, ErrNotInScan
	}
	stamp := ackNow()
	n := 0
	for _, it := range res.Items {
		if it.Verdict() != verdict || it.Acked() {
			continue
		}
		n++
		s.updateCachedMedia(it.MediaID, func(st *SeekStatus) { st.AckedAt = stamp })
	}
	if n > 0 {
		s.saveScans()
	}
	return n, nil
}

// updateCachedMedia is updateCachedFile keyed by media identity rather than
// path: the acknowledgement follows the title, whatever scope lists it.
func (s *SeekIndexService) updateCachedMedia(mediaID string, mut func(*SeekStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, c := range s.scans {
		changed := false
		for i := range c.res.Items {
			if c.res.Items[i].MediaID == mediaID {
				mut(&c.res.Items[i])
				changed = true
			}
		}
		if !changed {
			continue
		}
		recomputeRollups(&c.res)
		s.scans[key] = c
	}
}
