package fakes

import (
	"github.com/eveyproject/evey/internal/domain/plugin"
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

func (p *FakeManifestParser) ParseDir(_ string) (*plugin.PluginManifest, error) {
	if p.Err != nil {
		return nil, p.Err
	}
	return p.Result, nil
}
