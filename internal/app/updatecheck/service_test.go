package updatecheck

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/shared"
)

type fakeRegistry struct {
	remotes map[string]Remote
	tags    []string
	err     error
	calls   int
}

func (f *fakeRegistry) Describe(_ context.Context, _, tag string) (Remote, error) {
	f.calls++
	if f.err != nil {
		return Remote{}, f.err
	}
	r, ok := f.remotes[tag]
	if !ok {
		return Remote{}, ErrNotFound
	}
	return r, nil
}

func (f *fakeRegistry) Tags(context.Context, string) ([]string, error) {
	f.calls++
	return f.tags, f.err
}

type fakeCfg map[string]string

func (c fakeCfg) Get(_ context.Context, key string) (*shared.ServerConfigEntry, error) {
	v, ok := c[key]
	if !ok {
		return nil, errors.New("not found")
	}
	return &shared.ServerConfigEntry{Key: key, Value: v}, nil
}
func (c fakeCfg) Set(context.Context, string, string) error                 { return nil }
func (c fakeCfg) Delete(context.Context, string) error                      { return nil }
func (c fakeCfg) List(context.Context) ([]*shared.ServerConfigEntry, error) { return nil, nil }

const img = "docker.io/someone/evey"

func TestBranchBuild_FollowsItsOwnTag(t *testing.T) {
	build := Build{Version: "beta-0.0.1-full", Commit: "cbcd835ec767d3cbe4084bcce53e45aad1652eda", BuildDate: "2026-09-25T07:40:10Z"}
	reg := &fakeRegistry{remotes: map[string]Remote{
		"beta-0.0.1-full": {Revision: "cbcd835ec767d3cbe4084bcce53e45aad1652eda", Created: "2026-09-25T07:40:10Z"},
	}}
	s := New(reg, fakeCfg{}, build, img)
	assert.Equal(t, StateUpToDate, s.Check(context.Background()).State)

	reg.remotes["beta-0.0.1-full"] = Remote{Revision: "e77dc0a31aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Created: "2026-09-26T10:00:00Z"}
	st := s.Check(context.Background())
	assert.Equal(t, StateUpdateAvailable, st.State)
	assert.Equal(t, "e77dc0a31aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", st.LatestCommit)

	// A local build newer than the published one is not behind it.
	reg.remotes["beta-0.0.1-full"] = Remote{Revision: "0123456789", Created: "2026-09-24T10:00:00Z"}
	assert.Equal(t, StateUpToDate, s.Check(context.Background()).State)
}

func TestReleaseBuild_HearsOfAHigherReleaseOfItsVariantOnly(t *testing.T) {
	reg := &fakeRegistry{tags: []string{
		"1.2.3-full", "1.2.4-full", "1.3.0-beta.1-full", "2.0.0-nodejs", "1.2.5", "latest-full", "sha-abc1234-full",
	}, remotes: map[string]Remote{"1.2.4-full": {Revision: "abc"}}}
	s := New(reg, fakeCfg{}, Build{Version: "1.2.3-full", Commit: "x"}, img)
	st := s.Check(context.Background())
	assert.Equal(t, StateUpdateAvailable, st.State)
	assert.Equal(t, "1.2.4-full", st.LatestVersion, "a stable build ignores pre-releases and other variants")

	pre := New(reg, fakeCfg{}, Build{Version: "1.3.0-beta.0-full", Commit: "x"}, img)
	assert.Equal(t, "1.3.0-beta.1-full", pre.Check(context.Background()).LatestVersion)

	top := New(reg, fakeCfg{}, Build{Version: "1.2.4-full", Commit: "x"}, img)
	assert.Equal(t, StateUpToDate, top.Check(context.Background()).State)
}

func TestCheck_DisabledUnconfiguredAndDevBuildsDoNotAsk(t *testing.T) {
	reg := &fakeRegistry{}
	assert.Equal(t, StateDisabled, New(reg, fakeCfg{KeyEnabled: "false"}, Build{Version: "1.0.0"}, img).Check(context.Background()).State)
	assert.Equal(t, StateUnknown, New(reg, fakeCfg{}, Build{Version: "1.0.0"}, "").Check(context.Background()).State)
	assert.Equal(t, StateUnknown, New(reg, fakeCfg{}, Build{Version: "dev"}, img).Check(context.Background()).State)
	assert.Zero(t, reg.calls, "none of these may reach the registry")

	over := New(&fakeRegistry{tags: []string{"1.0.1"}}, fakeCfg{KeyImage: "ghcr.io/fork/evey"}, Build{Version: "1.0.0"}, img)
	assert.Equal(t, "ghcr.io/fork/evey", over.Check(context.Background()).Image)
}

func TestCheck_RegistryErrorIsUnknownNotUpToDate(t *testing.T) {
	s := New(&fakeRegistry{err: errors.New("dial tcp: timeout")}, fakeCfg{}, Build{Version: "beta-0.0.1-full", Commit: "abcdef0"}, img)
	st := s.Check(context.Background())
	assert.Equal(t, StateUnknown, st.State)
	assert.Contains(t, st.Reason, "timeout")
}

func TestCheckNow_DoesNotHammerTheRegistry(t *testing.T) {
	reg := &fakeRegistry{remotes: map[string]Remote{"beta-full": {Revision: "abcdef0"}}}
	s := New(reg, fakeCfg{}, Build{Version: "beta-full", Commit: "abcdef0"}, img)
	s.CheckNow(context.Background())
	s.CheckNow(context.Background())
	require.Equal(t, 1, reg.calls)
}

func TestSemverOrder(t *testing.T) {
	order := []string{"1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0", "1.0.1", "1.10.0"}
	for i := 1; i < len(order); i++ {
		a, _ := parseSemver(order[i-1])
		b, _ := parseSemver(order[i])
		assert.True(t, a.less(b), "%s < %s", order[i-1], order[i])
		assert.False(t, b.less(a), "%s > %s", order[i], order[i-1])
	}
	_, ok := parseSemver("beta-0.0.1")
	assert.False(t, ok, "a branch tag is not a version")
}
