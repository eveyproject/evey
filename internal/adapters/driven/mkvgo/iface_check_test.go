package mkvgo

import "github.com/eveyproject/evey/internal/domain/mkv"

// Compile-time interface checks: Adapter must satisfy MetadataEditor and
// SubtitleExtractor. This file adds no runtime tests - the compiler does the
// work.
var _ mkv.MetadataEditor = (*Adapter)(nil)
var _ mkv.SubtitleExtractor = (*Adapter)(nil)
