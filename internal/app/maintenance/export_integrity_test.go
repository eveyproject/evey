package maintenance

import "github.com/evey-project/evey/internal/domain/mkv"

// AudioIntegrityLostForTest exposes the comparison rule to the external test
// package, which pins it case by case.
func AudioIntegrityLostForTest(before, after []mkv.TrackIntegrity) error {
	return audioIntegrityLost(before, after)
}
