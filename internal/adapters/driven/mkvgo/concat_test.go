package mkvgo

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

func TestConcatCacheKey_DistinguishesChains(t *testing.T) {
	k1 := concatCacheKey([]string{"e1.mkv", "e2.mkv"}, apptranscoding.ConcatOptions{SegmentMs: 6000})
	k2 := concatCacheKey([]string{"e1.mkv", "e3.mkv"}, apptranscoding.ConcatOptions{SegmentMs: 6000})
	k3 := concatCacheKey([]string{"e1.mkv", "e2.mkv", "e3.mkv"}, apptranscoding.ConcatOptions{SegmentMs: 6000})
	assert.NotEqual(t, k1, k2, "a different chain must not alias")
	assert.NotEqual(t, k1, k3, "a longer chain must not alias its prefix")
}

func TestConcatPackager_Plan_RejectsSingleSource(t *testing.T) {
	p := NewConcatPackager()
	defer p.Close()
	_, err := p.Plan(t.Context(), []string{"one.mkv"}, apptranscoding.ConcatOptions{})
	assert.Error(t, err, "concat needs at least two sources - mkvgo's own guard, exercised through the adapter")
}
