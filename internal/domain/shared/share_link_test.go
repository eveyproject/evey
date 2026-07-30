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

// MaxViews is a POINTER: nil means unlimited, so a non-nil zero is a distinct,
// reachable state - a link that is created already exhausted. Only negatives are
// rejected, and mutation testing showed that `< 0` could become `<= 0` with the
// whole suite still green, which would make a zero-view link impossible to
// create at all.
//
// This pins the CURRENT contract rather than changing it: if zero should be
// rejected, that is a product decision, and this test is where it would be made
// visible.
func TestShareLinkValidate_MaxViewsBoundary(t *testing.T) {
	now := time.Now()
	link := func(maxViews *int) *ShareLink {
		return &ShareLink{
			ID: "shr-1", Token: "tok", MediaItemID: "mi-1", CreatedBy: "usr-1",
			CreatedAt: now, ExpiresAt: now.Add(time.Hour), MaxViews: maxViews,
		}
	}
	zero, negative, positive := 0, -1, 5

	if err := link(nil).Validate(); err != nil {
		t.Errorf("nil MaxViews means unlimited and must validate: %v", err)
	}
	if err := link(&zero).Validate(); err != nil {
		t.Errorf("a zero view budget is a real (already-exhausted) link, got %v", err)
	}
	if err := link(&positive).Validate(); err != nil {
		t.Errorf("a positive budget must validate: %v", err)
	}
	if err := link(&negative).Validate(); err == nil {
		t.Error("a negative view budget must be rejected")
	}
}
