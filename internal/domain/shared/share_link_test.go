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

func TestShareLink_Validate(t *testing.T) {
	now := time.Now()
	valid := func() ShareLink {
		return ShareLink{ID: "sl-1", Token: "tok", MediaItemID: "mi-1", CreatedBy: "usr-1", ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	}
	t.Run("valid", func(t *testing.T) {
		l := valid()
		assert.NoError(t, l.Validate())
	})
	t.Run("empty token rejected", func(t *testing.T) {
		l := valid()
		l.Token = ""
		assert.Error(t, l.Validate())
	})
	t.Run("non-expiring rejected", func(t *testing.T) {
		l := valid()
		l.ExpiresAt = time.Time{}
		assert.Error(t, l.Validate())
	})
	t.Run("expiry before creation rejected", func(t *testing.T) {
		l := valid()
		l.ExpiresAt = now.Add(-time.Hour)
		assert.Error(t, l.Validate())
	})
	t.Run("negative max views rejected", func(t *testing.T) {
		l := valid()
		neg := -1
		l.MaxViews = &neg
		assert.Error(t, l.Validate())
	})
	t.Run("constructor rejects invalid", func(t *testing.T) {
		_, err := NewShareLink("", "tok", "mi-1", "usr-1", now.Add(time.Hour), nil, now)
		assert.Error(t, err)
	})
}

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
