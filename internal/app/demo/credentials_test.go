package demo_test

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/demo"
)

// fakeCredentials holds API keys by owner and invite links by creator.
type fakeCredentials struct {
	keys    map[shared.UserID][]*user.APIKeyModel
	invites map[string][]*user.InviteLink
}

func (f *fakeCredentials) ListAPIKeys(_ context.Context, id shared.UserID) ([]*user.APIKeyModel, error) {
	return f.keys[id], nil
}

func (f *fakeCredentials) DeleteAPIKey(_ context.Context, id string) error {
	for owner, ks := range f.keys {
		kept := ks[:0]
		for _, k := range ks {
			if k.ID != id {
				kept = append(kept, k)
			}
		}
		f.keys[owner] = kept
	}
	return nil
}

func (f *fakeCredentials) ListInvites(_ context.Context, creator string) ([]*user.InviteLink, error) {
	return f.invites[creator], nil
}

func (f *fakeCredentials) DeleteInvite(_ context.Context, id string) error {
	for creator, ls := range f.invites {
		kept := ls[:0]
		for _, l := range ls {
			if l.ID != id {
				kept = append(kept, l)
			}
		}
		f.invites[creator] = kept
	}
	return nil
}

// A key minted on the demo outlived its visitor and nobody could revoke it; a
// leftover invite let anyone register (RegisterWithInvite is public). The boot
// and every reset purge both, for the demo accounts only.
func TestPurgeCredentials_DeletesTheDemoAccountsKeysAndInvites(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	if err := h.boot.EnsureAdmin(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.boot.SeedAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	admin, _ := h.users.GetByUsername(ctx, demo.Username)
	famille, _ := h.users.GetByUsername(ctx, "famille")
	stranger := shared.UserID("someone-else")
	creds := &fakeCredentials{
		keys: map[shared.UserID][]*user.APIKeyModel{
			admin.ID:   {{ID: "k1"}, {ID: "k2"}},
			famille.ID: {{ID: "k3"}},
			stranger:   {{ID: "k4"}},
		},
		invites: map[string][]*user.InviteLink{
			string(admin.ID): {{ID: "i1"}},
			string(stranger): {{ID: "i2"}},
		},
	}
	h.boot.WithCredentialPurge(creds)

	if err := h.boot.PurgeCredentials(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(creds.keys[admin.ID]) + len(creds.keys[famille.ID]); n != 0 {
		t.Errorf("%d demo API keys left", n)
	}
	if len(creds.invites[string(admin.ID)]) != 0 {
		t.Error("the demo account's invite link survived: anyone could register with it")
	}
	if len(creds.keys[stranger]) != 1 || len(creds.invites[string(stranger)]) != 1 {
		t.Error("the purge reached beyond the demo accounts")
	}
}
