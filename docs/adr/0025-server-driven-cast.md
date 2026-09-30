# ADR 0025 - Server-driven cast with a custom Web Receiver

Date: 2026-07-25
Status: Accepted, amended 2026-09-30 (see Amendment)

## Context

Casting from a browser sender historically relied on the Chromium built-in
receiver, with the sender computing and flinging a direct media URL. In
practice, Chromium reported `duration=NaN` for these file-direct URLs, which
made the sender-side fling unreliable: the receiver had no trustworthy
duration to seek against or to report progress from, and playback state on
cast targets (including Chromecast-class TVs) desynced from the source of
truth held by the server.

Casting also needs to carry Evey-specific behavior that the default Chromium
receiver cannot provide: AC-3 audio decoding via the `ac3go` wasm decoder
(the TV's native decoder does not cover every source codec) and passthrough
decisions per device audio-capability profile.

## Decision

- Cast is piloted **server-side**: the sender calls `LoadApp` to hand off
  playback intent to the server, which drives the cast session instead of the
  sender computing and flinging a URL directly to a generic receiver.
- A **custom Web Receiver** application is registered under Cast app ID
  `D36A449B`, hosted at `cast.eveyproject.org`. It runs the same `ac3go` wasm
  decoder used elsewhere in the stack, so AC-3 sources play back on the TV
  without a passthrough/transcode fallback.
- Three senders are wired against this receiver: web, mobile, and TV.

## Consequences

- The server, not the sender, is the source of truth for cast playback state
  (position, duration, errors), which removes the `duration=NaN` class of
  desync between what the sender displays and what is actually playing on the
  cast target.
- Evey now owns and must maintain a Web Receiver deployment (`cast.eveyproject.org`)
  in addition to the server and the three sender clients - one more surface to
  keep in sync with codec/decoder changes (e.g. `ac3go` wasm updates).
- Device audio-capability profiles that misreport passthrough support (seen on
  at least one TV model) can still cause a stuck cast session; this is tracked
  and fixed per-device profile, not by this ADR.

## Amendment - 2026-09-30

Two points of the decision no longer describe the product:

- **The wasm AC-3 path on the receiver is off by default.** A low-end
  operator box was killed twice mid-film with that path in play
  (2026-07-30), and its decoder measured 3.13x real time, so the path bought
  nothing: the AAC rendition plays the same film with sound. The receiver
  keeps the code, and `EVEY_CAST_WASM_AUDIO_ENABLED=true` turns it back on;
  without it a cast gets transcoded AAC, or AC-3/E-AC-3 passthrough when the
  receiver reports native Dolby support (`CastWasmAudioEnabled`,
  `internal/config/config.go`).
- **Two senders, not three.** The web and mobile apps cast; the TV app does
  not, and ships no Cast SDK.

The rest stands: the server drives the session, and the custom receiver
(`D36A449B`) is the one surface that reports the device's capabilities.
