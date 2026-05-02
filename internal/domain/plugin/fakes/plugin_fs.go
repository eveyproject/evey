package fakes

import "sync"

type FakePluginFileSystem struct {
	mu          sync.Mutex
	RemoveErr   error
	// RemoveErrTimes, when > 0, causes RemoveAll to return RemoveErr for the
	// first RemoveErrTimes calls, then succeed. 0 means always use RemoveErr
	// (original behaviour: fail forever if RemoveErr != nil).
	RemoveErrTimes int
	removeCallCount int
	Removed        []string
	TmpDirs        int
	TmpDirErr      error
	CopyErr        error
	CopiedStubs []string
	ChownedDirs []string
}

func NewFakePluginFileSystem() *FakePluginFileSystem {
	return &FakePluginFileSystem{}
}

func (f *FakePluginFileSystem) RemoveAll(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.RemoveErr != nil {
		if f.RemoveErrTimes == 0 || f.removeCallCount < f.RemoveErrTimes {
			f.removeCallCount++
			return f.RemoveErr
		}
	}
	f.removeCallCount++
	f.Removed = append(f.Removed, path)
	return nil
}

// RemoveCallCount returns the total number of RemoveAll calls made.
func (f *FakePluginFileSystem) RemoveCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.removeCallCount
}

func (f *FakePluginFileSystem) MkdirTemp(_, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.TmpDirErr != nil {
		return "", f.TmpDirErr
	}
	f.TmpDirs++
	return "/tmp/fake-plugin-tmpdir", nil
}

func (f *FakePluginFileSystem) CopySDKStubs(language, installDir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.CopyErr != nil {
		return f.CopyErr
	}
	f.CopiedStubs = append(f.CopiedStubs, installDir)
	return nil
}

func (f *FakePluginFileSystem) ChownPluginDir(dir string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ChownedDirs = append(f.ChownedDirs, dir)
}
