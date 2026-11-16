package plugin_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appplugin "github.com/eveyproject/evey/internal/app/plugin"
	plugindomain "github.com/eveyproject/evey/internal/domain/plugin"
	plgfakes "github.com/eveyproject/evey/internal/domain/plugin/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// countingAnalyzer counts the passes over a plugin's files.
type countingAnalyzer struct {
	inner *plgfakes.FakePluginAnalyzer
	runs  atomic.Int32
}

func (c *countingAnalyzer) Analyze(dir, webRoot string) (*plugindomain.AnalysisReport, error) {
	c.runs.Add(1)
	return c.inner.Analyze(dir, webRoot)
}

func newCountingInstaller(t *testing.T) (*appplugin.InstallerService, *plgfakes.FakeManifestParser, *countingAnalyzer, *plgfakes.FakePluginRepository) {
	t.Helper()
	repo := plgfakes.NewFakePluginRepository()
	parser := plgfakes.NewFakeManifestParser()
	analyzer := &countingAnalyzer{inner: plgfakes.NewFakePluginAnalyzer()}
	svc := appplugin.NewInstallerService(repo, plgfakes.NewFakePluginRunner(), parser, sharedfakes.NewFakeEventBus(),
		appplugin.InstallerWithInstaller(plgfakes.NewFakePluginInstaller()),
		appplugin.InstallerWithAnalyzer(analyzer),
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(plgfakes.NewFakePluginFileSystem()),
	)
	return svc, parser, analyzer, repo
}

// The install that follows a preview of the same zip keeps the preview's
// verdict instead of reading every file a second time.
func TestInstall_ReusesTheAnalysisOfThePreviewedZip(t *testing.T) {
	svc, parser, analyzer, _ := newCountingInstaller(t)
	parser.Result = goManifest("previewed")
	analyzer.inner.Report = &plugindomain.AnalysisReport{
		Safe:        true,
		FileCount:   7,
		RawFindings: []plugindomain.PluginFinding{{Category: "base64_blob", Severity: "INFO", File: "a.js", Line: 3}},
	}
	zip := tmpZip(t, "the previewed bytes")

	_, err := svc.PreviewPlugin(context.Background(), zip)
	require.NoError(t, err)
	require.EqualValues(t, 1, analyzer.runs.Load())

	p, err := svc.InstallPlugin(context.Background(), zip)
	require.NoError(t, err)
	assert.EqualValues(t, 1, analyzer.runs.Load(), "the install read the files again")
	require.NotNil(t, p.AnalysisReport, "the install keeps the report for the admin")
	assert.Equal(t, 7, p.AnalysisReport.FileCount)
	assert.Len(t, p.AnalysisReport.RawFindings, 1)
}

// A verdict belongs to the bytes it was reached on: another zip is read.
func TestInstall_AnotherZipThanThePreviewedOneIsAnalysed(t *testing.T) {
	svc, parser, analyzer, _ := newCountingInstaller(t)
	parser.Result = goManifest("swapped")

	_, err := svc.PreviewPlugin(context.Background(), tmpZip(t, "the previewed bytes"))
	require.NoError(t, err)
	// What the second pass finds is what decides, not the preview's clean report.
	analyzer.inner.Report = &plugindomain.AnalysisReport{Safe: false, Issues: []string{"[CRITICAL] evil.js:1 - reverse shell"}}

	_, err = svc.InstallPlugin(context.Background(), tmpZip(t, "other bytes"))
	require.Error(t, err)
	var blocked *appplugin.PluginBlockedError
	require.ErrorAs(t, err, &blocked)
	assert.EqualValues(t, 2, analyzer.runs.Load())
}

// A refusal reached at the preview is the install's refusal too.
func TestInstall_ARefusalAtThePreviewStillBlocksTheInstall(t *testing.T) {
	svc, parser, analyzer, repo := newCountingInstaller(t)
	parser.Result = goManifest("refused")
	analyzer.inner.Report = &plugindomain.AnalysisReport{Safe: false, Issues: []string{"[CRITICAL] evil.js:1 - reverse shell"}}
	zip := tmpZip(t, "malicious bytes")

	_, err := svc.PreviewPlugin(context.Background(), zip)
	require.NoError(t, err)
	// A report that would now pass must not be asked for: the kept one stands.
	analyzer.inner.Report = &plugindomain.AnalysisReport{Safe: true}

	_, err = svc.InstallPlugin(context.Background(), zip)
	var blocked *appplugin.PluginBlockedError
	require.ErrorAs(t, err, &blocked)
	assert.EqualValues(t, 1, analyzer.runs.Load())
	all, listErr := repo.List(context.Background())
	require.NoError(t, listErr)
	assert.Empty(t, all)
}

// What an install adds to its own copy of the report never reaches the kept
// one: an upgrade to the same zip starts again from the preview's findings.
func TestInstall_TheKeptAnalysisIsNotAlteredByTheInstall(t *testing.T) {
	svc, parser, analyzer, _ := newCountingInstaller(t)
	m := goManifest("gpu-user")
	m.Permissions = []string{"hardware.gpu"}
	parser.Result = m
	analyzer.inner.Report = &plugindomain.AnalysisReport{Safe: true, Issues: []string{"[INFO] a.js:1 - note"}}
	zip := tmpZip(t, "gpu bytes")

	_, err := svc.PreviewPlugin(context.Background(), zip)
	require.NoError(t, err)
	first, err := svc.InstallPlugin(context.Background(), zip)
	require.NoError(t, err)
	require.Len(t, first.AnalysisReport.Issues, 2, "the GPU warning is appended at install")

	// The same bytes under a higher version (the fake parser reads no zip).
	next := goManifestVersioned("gpu-user", "1.0.1")
	next.Permissions = []string{"hardware.gpu"}
	parser.Result = next
	second, err := svc.InstallPlugin(context.Background(), zip)
	require.NoError(t, err)
	assert.Len(t, second.AnalysisReport.Issues, 2, "the warning piled up on the kept report")
	assert.EqualValues(t, 1, analyzer.runs.Load())
}

// A theme's theme.css is rewritten at install: its files are read as served,
// whatever the preview concluded on the raw ones.
func TestInstall_AThemeIsAlwaysAnalysedAsInstalled(t *testing.T) {
	svc, parser, analyzer, _ := newCountingInstaller(t)
	m := goManifest("a-theme")
	m.Kind = plugindomain.KindTheme
	m.Language, m.Entrypoint = "", ""
	parser.Result = m
	zip := tmpZip(t, "theme bytes")

	_, err := svc.PreviewPlugin(context.Background(), zip)
	require.NoError(t, err)
	_, err = svc.InstallPlugin(context.Background(), zip)
	require.NoError(t, err)
	assert.EqualValues(t, 2, analyzer.runs.Load())
}

// An analysis that failed at the preview is no verdict: the install runs its own.
func TestInstall_AFailedPreviewAnalysisIsNotKept(t *testing.T) {
	svc, parser, analyzer, _ := newCountingInstaller(t)
	parser.Result = goManifest("retried")
	zip := tmpZip(t, "retried bytes")

	analyzer.inner.Err = assert.AnError
	_, err := svc.PreviewPlugin(context.Background(), zip)
	require.NoError(t, err, "the preview tolerates a failed analysis")
	analyzer.inner.Err = nil

	_, err = svc.InstallPlugin(context.Background(), zip)
	require.NoError(t, err)
	assert.EqualValues(t, 2, analyzer.runs.Load())
}

// An upgrade does not extract over leftovers it could not clear.
func TestInstall_AnUpgradeStopsWhenItsStagingDirCannotBeCleared(t *testing.T) {
	repo := plgfakes.NewFakePluginRepository()
	parser := plgfakes.NewFakeManifestParser()
	fs := plgfakes.NewFakePluginFileSystem()
	svc := appplugin.NewInstallerService(repo, plgfakes.NewFakePluginRunner(), parser, sharedfakes.NewFakeEventBus(),
		appplugin.InstallerWithInstaller(plgfakes.NewFakePluginInstaller()),
		appplugin.InstallerWithAnalyzer(plgfakes.NewFakePluginAnalyzer()),
		appplugin.InstallerWithPluginDir("/tmp/test-plugins"),
		appplugin.InstallerWithFileSystem(fs),
	)
	parser.Result = goManifest("stuck")
	_, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1"))
	require.NoError(t, err)

	parser.Result = goManifestVersioned("stuck", "1.0.1")
	fs.RemoveErr = assert.AnError
	_, err = svc.InstallPlugin(context.Background(), tmpZip(t, "v2"))
	require.ErrorIs(t, err, assert.AnError)
	stored, getErr := repo.GetByName(context.Background(), "stuck")
	require.NoError(t, getErr)
	assert.Equal(t, "1.0.0", stored.Version)
}

// The automatic update judges a release, then installs it: one pass.
func TestAutoUpdate_AnAppliedReleaseIsAnalysedOnce(t *testing.T) {
	rig := newAutoRig(t, "warn")
	p := rig.installFromRepo(t, &rig.key)
	before := rig.hook.runs.Load()

	rig.publish(t, goManifestVersioned("thing", "1.0.1"), rig.signed(t, "v101"))
	st := rig.check(t, p.ID)

	require.Equal(t, plugindomain.UpdateApplied, st.Outcome, st.Reason)
	assert.EqualValues(t, 1, rig.hook.runs.Load()-before)
}

// An update applied after its own preview is the same pair as an install
// after a preview: one pass over the candidate.
func TestApplyUpdate_ReusesTheAnalysisOfItsPreview(t *testing.T) {
	svc, parser, analyzer, _ := newCountingInstaller(t)
	parser.Result = goManifestVersioned("updated", "1.0.0")
	p, err := svc.InstallPlugin(context.Background(), tmpZip(t, "v1 bytes"))
	require.NoError(t, err)
	before := analyzer.runs.Load()

	parser.Result = goManifestVersioned("updated", "1.1.0")
	candidate := tmpZip(t, "v2 bytes")
	preview, err := svc.PreviewUpdate(context.Background(), p.ID, candidate)
	require.NoError(t, err)
	updated, err := svc.ApplyUpdate(context.Background(), p.ID, candidate, preview.SourceHash, false)
	require.NoError(t, err)

	assert.Equal(t, "1.1.0", updated.Version)
	assert.EqualValues(t, 1, analyzer.runs.Load()-before)
}
