package fakes

type FakePluginInstaller struct {
	Err       error
	Extracted map[string]string // zipPath -> destDir
}

func NewFakePluginInstaller() *FakePluginInstaller {
	return &FakePluginInstaller{Extracted: make(map[string]string)}
}

func (f *FakePluginInstaller) Extract(zipPath, destDir string) error {
	if f.Err != nil {
		return f.Err
	}
	f.Extracted[zipPath] = destDir
	return nil
}
