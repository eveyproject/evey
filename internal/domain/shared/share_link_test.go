package shared

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestShareLink_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"past", time.Now().Add(-time.Hour), true},
		{"future", time.Now().Add(time.Hour), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := &ShareLink{ExpiresAt: tt.expiresAt}
			assert.Equal(t, tt.want, link.IsExpired())
		})
	}
}

func intPtr(v int) *int { return &v }

func TestShareLink_IsViewLimitReached(t *testing.T) {
	tests := []struct {
		name      string
		maxViews  *int
		viewCount int
		want      bool
	}{
		{"no limit", nil, 100, false},
		{"under limit", intPtr(5), 3, false},
		{"at limit", intPtr(5), 5, true},
		{"over limit", intPtr(5), 7, true},
		{"zero limit", intPtr(0), 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := &ShareLink{MaxViews: tt.maxViews, ViewCount: tt.viewCount}
			assert.Equal(t, tt.want, link.IsViewLimitReached())
		})
	}
}
