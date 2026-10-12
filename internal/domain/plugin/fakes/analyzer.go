package fakes

import "github.com/eveyproject/evey/internal/domain/plugin"

type FakePluginAnalyzer struct {
	Report *plugin.AnalysisReport
	Err    error
}

func NewFakePluginAnalyzer() *FakePluginAnalyzer {
	return &FakePluginAnalyzer{
		Report: &plugin.AnalysisReport{Safe: true},
	}
}

func (f *FakePluginAnalyzer) Analyze(_, _ string) (*plugin.AnalysisReport, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Report, nil
}
