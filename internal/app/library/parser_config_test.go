package library_test

import (
	"context"
	"testing"

	applibrary "github.com/eveyproject/evey/internal/app/library"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The decoupling (ParserTokenApplier injected, not the filesystem package
// called directly) is what makes this testable at all.
func TestApplyParserConfig_InjectsSavedTokens(t *testing.T) {
	repo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, repo.Set(context.Background(), applibrary.ParserConfigKey,
		`{"extra_qualities":["REMUX-X"],"extra_edition_suffixes":["DIRECTORS"]}`))

	var gotQ, gotE []string
	applibrary.ApplyParserConfig(context.Background(), repo, func(q, e []string) {
		gotQ, gotE = q, e
	})

	assert.Equal(t, []string{"REMUX-X"}, gotQ)
	assert.Equal(t, []string{"DIRECTORS"}, gotE)
}

func TestApplyParserConfig_NoConfig_AppliesEmpty(t *testing.T) {
	repo := sharedfakes.NewFakeServerConfigRepository()
	called := false
	applibrary.ApplyParserConfig(context.Background(), repo, func(q, e []string) {
		called = true
		assert.Empty(t, q)
		assert.Empty(t, e)
	})
	assert.True(t, called, "applier should still be invoked with empty tokens")
}
