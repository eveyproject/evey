package grpcapi

import (
	"testing"

	"github.com/stretchr/testify/assert"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
)

// A parent's children (a season's episodes, a series' seasons) are served by
// number unless the client asks otherwise. Every client lists a season with
// ListMediaItems(parentId) and no sort; by title, a season longer than a page
// came back as its first 500 titles, not its first 500 episodes.
func TestMediaItemsFilterFromRequest_ChildrenDefaultToIndexNumber(t *testing.T) {
	t.Run("parent without a sort", func(t *testing.T) {
		f := mediaItemsFilterFromRequest(&eveyv1.ListMediaItemsRequest{ParentId: "season-1"})
		assert.Equal(t, "index_number", f.SortField)
	})
	t.Run("an explicit sort is kept", func(t *testing.T) {
		f := mediaItemsFilterFromRequest(&eveyv1.ListMediaItemsRequest{
			ParentId: "season-1", SortField: eveyv1.MediaSortField_MEDIA_SORT_FIELD_PREMIERE_DATE,
		})
		assert.Equal(t, "premiere_date", f.SortField)
	})
	t.Run("a library listing keeps the title default", func(t *testing.T) {
		f := mediaItemsFilterFromRequest(&eveyv1.ListMediaItemsRequest{LibraryId: "lib-1"})
		assert.Empty(t, f.SortField)
	})
}
