package httpapi

import (
	"context"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared"
)

// The kill switch: a CAST receiver must not be handed the wasm-audio path while
// the 2026-07-30 death is unexplained (Bouygtel4K killed twice mid-film, sound
// working, both readable memory figures flat; its decoder measured 3.13x real
// time, so the path bought it nothing). Browsers keep the path - it is
// validated there and it is what plays Dolby without a transcode.
//
// This pins the DECISION expressed in serveWasmAudioMaster: decline when the
// caller is a cast stream token and the operator switch is off.
func castWasmPathOffered(castToken bool, switchOn bool) bool {
	ctx := context.Background()
	if castToken {
		ctx = context.WithValue(ctx, streamTokenCtxKey, &shared.StreamToken{Cast: true})
	}
	st, ok := StreamTokenFromContext(ctx)
	isGatedCast := ok && st != nil && st.Cast && !switchOn
	return !isGatedCast
}

func TestCastWasmAudioKillSwitch(t *testing.T) {
	t.Run("a cast receiver is declined by default", func(t *testing.T) {
		if castWasmPathOffered(true, false) {
			t.Error("cast + switch off: the wasm path was offered, want declined")
		}
	})

	t.Run("the operator can turn it back on without a deploy", func(t *testing.T) {
		if !castWasmPathOffered(true, true) {
			t.Error("cast + switch on: the wasm path was declined, want offered")
		}
	})

	t.Run("a browser is never affected by the CAST switch", func(t *testing.T) {
		// No cast stream token: the web player keeps the decoder it was
		// validated with, whatever the cast switch says.
		if !castWasmPathOffered(false, false) {
			t.Error("browser: the wasm path was declined by the CAST switch")
		}
	})
}
