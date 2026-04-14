package fakes

import "github.com/evey-project/evey/internal/domain/library"

type FakeSubtitleScanner struct {
	Results map[string][]library.ExternalSubtitle
	Err     error
}

func NewFakeSubtitleScanner() *FakeSubtitleScanner {
	return &FakeSubtitleScanner{Results: make(map[string][]library.ExternalSubtitle)}
}

func (s *FakeSubtitleScanner) ScanForSubtitles(videoPath string) ([]library.ExternalSubtitle, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Results[videoPath], nil
}
