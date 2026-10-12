package httpapi

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// The master handler and the segment fingerprint MUST agree on whether the
// wasm-audio path is in play. When they disagreed on 2026-07-29 the master
// advertised muxed audio over audio-free segments and the Chromecast answered
// 104 then 905; the cast kill switch reopened the exact same hole the same
// evening, within an hour of shipping. wasmAudioEffective is the single answer
// every decision now reads - this test exists so a third condition cannot be
// added to one side alone again.
func TestWasmAudioEffective(t *testing.T) {
	cases := []struct {
		name        string
		wasmAudio   bool
		fromCast    bool
		castEnabled bool
		want        bool
	}{
		{"a browser asking for wasm keeps it", true, false, false, true},
		{"a cast asking for wasm is refused while the switch is off", true, true, false, false},
		{"a cast gets it back when the operator turns the switch on", true, true, true, true},
		{"nobody asked, from a cast: never in play", false, true, true, false},
		{"nobody asked, from a browser: never in play", false, false, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := "/api/v1/media/m1/hls/master.m3u8"
			if tc.wasmAudio {
				url += "?wasmAudio=1"
			}
			r := httptest.NewRequest("GET", url, nil)
			if tc.fromCast {
				r = r.WithContext(context.WithValue(r.Context(), streamTokenCtxKey, &shared.StreamToken{Cast: true}))
			}
			if got := wasmAudioEffective(r, tc.castEnabled); got != tc.want {
				t.Errorf("wasmAudioEffective = %v, want %v", got, tc.want)
			}
		})
	}
}
