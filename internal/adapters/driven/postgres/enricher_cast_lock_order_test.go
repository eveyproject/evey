package postgres_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	"github.com/eveyproject/evey/internal/adapters/driven/postgres/testhelpers"
	appmetadata "github.com/eveyproject/evey/internal/app/metadata"
	domainmetadata "github.com/eveyproject/evey/internal/domain/metadata"
)

// Two enrichments of titles sharing their cast, billed in opposite orders,
// run at once: each person write holds a per-id lock to the end of its
// transaction, and taken in billing order the locks crossed and Postgres
// failed one enrichment as a deadlock. Resolved in provider-id order, both
// commit.
func TestEnrichItem_CrossedCastsDoNotDeadlock(t *testing.T) {
	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()
	ctx := context.Background()

	libRepo := postgres.NewLibraryRepository(db)
	itemRepo := postgres.NewMediaItemRepository(db)
	lib := newTestLibrary("castlock")
	require.NoError(t, libRepo.Create(ctx, lib))
	enricher := appmetadata.NewEnricher(
		postgres.NewArtworkRepository(db), postgres.NewPersonRepository(db), postgres.NewMediaPersonRepository(db),
		postgres.NewMediaItemQueryRepoAdapter(db), itemRepo, nil,
		nil, nil, nil, nil,
		appmetadata.EnricherWithUnitOfWork(postgres.NewTxUnit(db)),
	)

	for round := range 8 {
		cast := make([]domainmetadata.PersonResult, 0, 12)
		for i := range 12 {
			cast = append(cast, domainmetadata.PersonResult{
				Name: fmt.Sprintf("Actor %d-%d", round, i), Role: "Actor", ProviderID: fmt.Sprintf("%d%03d", round+1, i),
			})
		}
		reversed := slices.Clone(cast)
		slices.Reverse(reversed)
		a := newTestMediaItem(fmt.Sprintf("castlock-a-%d", round), lib.ID)
		b := newTestMediaItem(fmt.Sprintf("castlock-b-%d", round), lib.ID)
		require.NoError(t, itemRepo.Create(ctx, a))
		require.NoError(t, itemRepo.Create(ctx, b))

		var wg sync.WaitGroup
		gate := make(chan struct{})
		errs := make([]error, 2)
		wg.Go(func() {
			<-gate
			errs[0] = enricher.EnrichItem(ctx, a, &domainmetadata.MetadataResult{Title: "A", People: cast})
		})
		wg.Go(func() {
			<-gate
			errs[1] = enricher.EnrichItem(ctx, b, &domainmetadata.MetadataResult{Title: "B", People: reversed})
		})
		close(gate)
		wg.Wait()
		require.NoError(t, errs[0], "round %d", round)
		require.NoError(t, errs[1], "round %d", round)
	}
}
