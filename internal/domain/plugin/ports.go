package plugin

import "context"

type PluginRepository interface {
	Create(ctx context.Context, p *Plugin) error
	GetByID(ctx context.Context, id string) (*Plugin, error)
	GetByName(ctx context.Context, name string) (*Plugin, error)
	List(ctx context.Context) ([]*Plugin, error)
	Update(ctx context.Context, p *Plugin) error
	Delete(ctx context.Context, id string) error
}

type PluginRuntimeRepository interface {
	Create(ctx context.Context, rt *PluginRuntime) error
	GetByLanguageAndVersion(ctx context.Context, language, version string) (*PluginRuntime, error)
	ListByLanguage(ctx context.Context, language string) ([]*PluginRuntime, error)
	List(ctx context.Context) ([]*PluginRuntime, error)
	Delete(ctx context.Context, id string) error
}

type PluginRunner interface {
	Start(ctx context.Context, p *Plugin) error
	Stop(ctx context.Context, pluginID string) error
	Health(ctx context.Context, pluginID string) (bool, error)
	SendEvent(ctx context.Context, pluginID string, event any) error
}

type ManifestParser interface {
	Parse(zipPath string) (*PluginManifest, error)
	ParseDir(dir string) (*PluginManifest, error)
}

type PluginInstaller interface {
	Extract(zipPath, destDir string) error
}

type PluginLogger interface {
	WriteLog(pluginID, line string)
}

type PluginAnalyzer interface {
	Analyze(dir string) (*AnalysisReport, error)
}

type WidgetCompiler interface {
	CompileWidget(srcPath, outPath string) error
}

// CSSSanitizer removes dangerous CSS constructs from plugin themes.
type CSSSanitizer interface {
	SanitizeCSS(input []byte) ([]byte, []string)
}

// PluginFileSystem abstracts filesystem operations for plugin install/uninstall.
type PluginFileSystem interface {
	RemoveAll(path string) error
	MkdirTemp(dir, pattern string) (string, error)
	CopySDKStubs(language, installDir string) error
	ChownPluginDir(dir string)
}
