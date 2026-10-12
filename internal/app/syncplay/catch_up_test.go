package syncplay

// Catching up to the room, without dragging the room to you.
//
// The only cross-member control a member had was "resync", which means
// "everyone sync to ME" - it moves everybody's picture. Someone who has simply
// drifted, or who paused for a minute, had no way to rejoin the group quietly:
// leaving the room and coming back was the entire procedure.
//
// catch_up answers the asking member alone with the room's LIVE position and
// broadcasts nothing, so a viewer at the back can slip back into step without
// anyone else noticing.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/syncplay"
	"github.com/eveyproject/evey/internal/domain/syncplay/fakes"
)

func lastCommand(t *testing.T, s *fakes.FakeMessageSender) syncplay.SyncCommand {
	t.Helper()
	msgs := s.Received()
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	var cmd syncplay.SyncCommand
	if err := json.Unmarshal(msgs[len(msgs)-1], &cmd); err != nil {
		t.Fatalf("decode message: %v", err)
	}
	return cmd
}

func TestHub_CatchUp_AnswersTheAskerAloneWithTheLivePosition(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, err := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	host := fakes.NewFakeMessageSender()
	guest := fakes.NewFakeMessageSender()
	if _, err := h.ConnectMember(ctx, room.ID, "host-1", "Host", host); err != nil {
		t.Fatalf("ConnectMember host: %v", err)
	}
	if _, err := h.ConnectMember(ctx, room.ID, "guest-1", "Guest", guest); err != nil {
		t.Fatalf("ConnectMember guest: %v", err)
	}

	// The room has been playing for ten seconds from a minute in.
	if err := h.repo.UpdateState(ctx, room.ID, syncplay.PlaybackState{
		Playing: true, PositionMs: 60_000, UpdatedAt: time.Now().UTC().Add(-10 * time.Second),
	}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	host.Messages = nil
	guest.Messages = nil

	if _, err := h.HandleCommand(ctx, room.ID, "guest-1", "Guest",
		syncplay.SyncCommand{Type: "catch_up"}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}

	got := lastCommand(t, guest)
	if got.Type != "state" {
		t.Errorf("type = %q, want state", got.Type)
	}
	if !got.Playing {
		t.Error("the room is playing, so the answer must say so")
	}
	// A LIVE position: the stored snapshot plus the time that has passed.
	if got.PositionMs < 68_000 || got.PositionMs > 72_000 {
		t.Errorf("position = %dms, want ~70000 (the live position, not the stale snapshot)", got.PositionMs)
	}

	// The whole difference from "resync": nobody else's player moves.
	if n := len(host.Received()); n != 0 {
		t.Errorf("the rest of the room received %d messages, want 0", n)
	}
}

// A room that is paused answers with its stored position, unadvanced: nothing
// has been playing, so nothing has moved on.
func TestHub_CatchUp_APausedRoomDoesNotAdvance(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	guest := fakes.NewFakeMessageSender()
	if _, err := h.ConnectMember(ctx, room.ID, "guest-1", "Guest", guest); err != nil {
		t.Fatalf("ConnectMember: %v", err)
	}
	if err := h.repo.UpdateState(ctx, room.ID, syncplay.PlaybackState{
		Playing: false, PositionMs: 42_000, UpdatedAt: time.Now().UTC().Add(-30 * time.Second),
	}); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	guest.Messages = nil

	if _, err := h.HandleCommand(ctx, room.ID, "guest-1", "Guest",
		syncplay.SyncCommand{Type: "catch_up"}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	got := lastCommand(t, guest)
	if got.PositionMs != 42_000 {
		t.Errorf("position = %dms, want 42000 unchanged while paused", got.PositionMs)
	}
	if got.Playing {
		t.Error("a paused room must not tell a joiner to play")
	}
}

// Someone who is not a member of the room learns nothing from it.
func TestHub_CatchUp_IgnoresANonMember(t *testing.T) {
	t.Parallel()
	h := newTestHub()
	ctx := context.Background()

	room, _ := h.CreateRoom(ctx, "host-1", "media-1", "Room")
	if _, err := h.ConnectMember(ctx, room.ID, "host-1", "Host", fakes.NewFakeMessageSender()); err != nil {
		t.Fatalf("ConnectMember: %v", err)
	}

	if _, err := h.HandleCommand(ctx, room.ID, "stranger", "Stranger",
		syncplay.SyncCommand{Type: "catch_up"}); err != nil {
		t.Fatalf("HandleCommand: %v", err)
	}
	// Nothing to assert on a sender the stranger never had: the point is that
	// the call is a no-op and does not panic or broadcast.
}
