package main

import (
	"context"

	applibrary "github.com/eveyproject/evey/internal/app/library"
	appmaintenance "github.com/eveyproject/evey/internal/app/maintenance"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// seekIndexCatalog adapts the library QueryService to the maintenance
// MediaCatalog port: the seek-index scanner only needs each item's id, title
// and on-disk path.
type seekIndexCatalog struct{ lib *applibrary.QueryService }

// seekIndexCatalogPageSize matches the repo's DefaultListLimit so each page is a
// single round-trip; ListMediaItems caps every query at that many rows, so we
// must paginate to cover a library of thousands of files (a single call would
// silently scan only the first page).
const seekIndexCatalogPageSize = 500

func (c seekIndexCatalog) ListMediaFiles(ctx context.Context, libraryID string) ([]appmaintenance.MediaRef, error) {
	var refs []appmaintenance.MediaRef
	for offset := 0; ; offset += seekIndexCatalogPageSize {
		filter := library.ListMediaItemsFilter{Limit: seekIndexCatalogPageSize, Offset: offset}
		if libraryID != "" {
			lid := shared.LibraryID(libraryID)
			filter.LibraryID = &lid
		}
		items, err := c.lib.ListMediaItems(ctx, filter)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if it == nil || it.File == nil {
				continue
			}
			refs = append(refs, appmaintenance.MediaRef{
				MediaID: string(it.ID),
				Title:   it.Title,
				Path:    it.File.Path,
			})
		}
		if len(items) < seekIndexCatalogPageSize {
			break
		}
	}
	return refs, nil
}

func (c seekIndexCatalog) GetMediaFile(ctx context.Context, mediaID string) (appmaintenance.MediaRef, error) {
	it, err := c.lib.GetMediaItem(ctx, mediaID)
	if err != nil {
		return appmaintenance.MediaRef{}, err
	}
	ref := appmaintenance.MediaRef{MediaID: string(it.ID), Title: it.Title}
	if it.File != nil {
		ref.Path = it.File.Path
	}
	return ref, nil
}

// seekIndexRelocator adapts the library CommandService to the maintenance
// MediaRelocator port: after the wrong-container repair renames a file, the
// catalog entry follows it.
type seekIndexRelocator struct{ cmd *applibrary.CommandService }

func (r seekIndexRelocator) RelocateMediaFile(ctx context.Context, mediaID, newPath string) error {
	return r.cmd.RelocateMediaFile(ctx, mediaID, newPath)
}
