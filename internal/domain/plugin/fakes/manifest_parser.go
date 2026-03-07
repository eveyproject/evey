package fakes

import (
	"github.com/gravity-zero/evey/internal/domain/plugin"
)

type FakeManifestParser struct {
	Result *plugin.PluginManifest
	Err    error
}

func NewFakeManifestParser() *FakeManifestParser {
	return &FakeManifestParser{}
}

func (p *FakeManifestParser) Parse(_ string) (*plugin.PluginManifest, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	return p.Result, nil
}
