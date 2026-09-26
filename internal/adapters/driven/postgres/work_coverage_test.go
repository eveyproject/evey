package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/evey-project/evey/internal/pkg/bgtasks"
)

// assertCoveragePending checks a system-health count against its worker's own
// list: what the screen says is left is what the worker will pick up.
func assertCoveragePending(t *testing.T, c bgtasks.Coverage, pending int) {
	t.Helper()
	assert.Equal(t, int64(pending), c.TotalItems-c.DoneItems, "pending items (done %d of %d)", c.DoneItems, c.TotalItems)
}
