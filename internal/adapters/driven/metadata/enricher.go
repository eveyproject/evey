package metadata

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	domaindomain "github.com/gravity-zero/evey/internal/domain/library"
	domainmetadata "github.com/gravity-zero/evey/internal/domain/metadata"
)

// Enricher stores metadata results (artwork, people) into the DB.
type Enricher struct {
	artwork      domainmetadata.ArtworkRepository
	persons      domaindomain.PersonRepository
	mediaPersons domaindomain.MediaPersonRepository
	mediaItems   domaindomain.MediaItemRepository
}

func NewEnricher(
	artwork domainmetadata.ArtworkRepository,
	persons domaindomain.PersonRepository,
	mediaPersons domaindomain.MediaPersonRepository,
	mediaItems domaindomain.MediaItemRepository,
) *Enricher {
	return &Enricher{
		artwork:      artwork,
		persons:      persons,
		mediaPersons: mediaPersons,
		mediaItems:   mediaItems,
	}
}

// EnrichItem applies a MetadataResult to a media item: updates the item,
// replaces artwork records, and upserts people/cast.
func (e *Enricher) EnrichItem(ctx context.Context, item *domaindomain.MediaItem, result *domainmetadata.MetadataResult) error {
	item.Title = result.Title
	item.OriginalTitle = result.OriginalTitle
	item.Overview = result.Overview
	item.Tagline = result.Tagline
	item.Genres = result.Genres
	item.Studios = result.Studios
	item.PremiereDate = result.PremiereDate
	item.Year = result.Year
	item.CommunityRating = result.CommunityRating
	item.OfficialRating = result.OfficialRating
	item.UpdatedAt = time.Now()

	if item.ProviderIDs == nil {
		item.ProviderIDs = make(map[string]string)
	}
	for k, v := range result.ProviderIDs {
		item.ProviderIDs[k] = v
	}

	if err := e.mediaItems.Update(ctx, item); err != nil {
		return fmt.Errorf("update media item: %w", err)
	}

	if err := e.artwork.DeleteByMediaItemID(ctx, item.ID); err != nil {
		return fmt.Errorf("delete old artwork: %w", err)
	}
	for i, art := range result.Artwork {
		src := art.URL
		lang := art.Language
		w, h := art.Width, art.Height
		rec := &domainmetadata.ArtworkRecord{
			ID:          uuid.New().String(),
			MediaItemID: item.ID,
			Type:        art.Type,
			Provider:    "tmdb",
			SourceURL:   &src,
			SortOrder:   i,
		}
		if lang != "" {
			rec.Language = &lang
		}
		if w > 0 {
			rec.Width = &w
		}
		if h > 0 {
			rec.Height = &h
		}
		if err := e.artwork.Create(ctx, rec); err != nil {
			return fmt.Errorf("create artwork: %w", err)
		}
	}

	if err := e.mediaPersons.DeleteByMediaItemID(ctx, item.ID); err != nil {
		return fmt.Errorf("delete old media_people: %w", err)
	}
	for i, p := range result.People {
		person, err := e.persons.GetByName(ctx, p.Name)
		if err != nil {
			if !errors.Is(err, domaindomain.ErrNotFound) {
				return fmt.Errorf("get person by name: %w", err)
			}
			thumbPath := p.ThumbURL
			var thumbPtr *string
			if thumbPath != "" {
				thumbPtr = &thumbPath
			}
			person = &domaindomain.Person{
				ID:          uuid.New().String(),
				Name:        p.Name,
				ProviderIDs: map[string]string{},
				ThumbPath:   thumbPtr,
			}
			if err := e.persons.Create(ctx, person); err != nil {
				return fmt.Errorf("create person: %w", err)
			}
		}

		var charName *string
		if p.Character != "" {
			c := p.Character
			charName = &c
		}
		mp := &domaindomain.MediaPerson{
			MediaItemID:   item.ID,
			PersonID:      person.ID,
			RoleType:      roleType(p.Role),
			CharacterName: charName,
			SortOrder:     i,
		}
		if createErr := e.mediaPersons.Create(ctx, mp); createErr != nil {
			// ignore duplicate (person already linked with same role)
			_ = createErr
		}
	}

	return nil
}

func roleType(role string) string {
	switch role {
	case "Actor":
		return "actor"
	case "Director":
		return "director"
	case "Writer":
		return "writer"
	case "Producer":
		return "producer"
	default:
		return "actor"
	}
}

