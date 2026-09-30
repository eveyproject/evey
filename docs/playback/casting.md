# Casting

The **Cast** button, in the web player in Chrome and in the Android app, sends
the film to a Chromecast or a TV with Chromecast built in. The phone or the
browser only starts the session: the TV plays the stream from the server
directly, through Evey's own receiver application, and the server decides what
to send for the TV, not for the device that cast.

## What the server knows about the receiver

The stream is decoded by the TV, so the playback decision uses the receiver's
capabilities, from the most reliable source available:

1. **What the receiver observed on itself.** Evey's receiver tests its own
   device when it starts (which codecs and resolutions it can display, whether
   it displays HDR, whether it passes Dolby audio through, how many audio
   channels its output carries) and reports it. When this report arrives, it is
   the one used.
2. **Otherwise, the model.** The name the Cast SDK reports (Chromecast with
   Google TV, Chromecast Ultra, Google TV Streamer...) maps to the documented
   capabilities of that family.
3. **Otherwise, a conservative profile.** An unknown device, a receiver that
   did not answer in time, or a device found without a model name gets the
   capabilities of the oldest Chromecast.

The order follows the cost of a mistake: assuming a capability the device does
not have gives a black screen, while assuming too little only costs a
transcode.

Dolby Digital (AC-3, E-AC-3) is sent as it is only to a receiver that reported
passing it through natively; otherwise it is re-encoded to AAC.

## The TV's connection

Before asking for a stream, the receiver measures its own throughput and sends
it with the request. The server keeps the original video when that throughput
carries it with a 20 % margin. Otherwise it chooses the highest quality the
connection carries. If playback stalls anyway, the receiver reloads once on the
lowest quality, provided that halves the bitrate. Stream info then shows **The
line the client measured cannot carry the copy** or **Survival rung asked for
by the player**.

Older cast devices do not switch quality on their own during playback; this is
why the server makes that choice for them.

## When a cast does not work

[A cast froze, went silent, or the phone keeps spinning](../runbooks/incident-runbook.md#8-a-cast-froze-went-silent-or-the-phone-keeps-spinning).

<!-- sources
receiver profile order: frontend/packages/media-caps/src/castUrl.ts - castProfileFor, profileFromReceiverCaps, pickCastProfile, FLOOR_PROFILE
receiver probe: frontend/apps/web/src/cast-receiver/receiverLogic.ts - probeReceiverCaps
no wasm on the receiver: frontend/apps/web/src/cast-receiver/main.ts - CAST_WASM_ENABLED
line and floor: internal/app/transcoding/resolver_line.go
senders: frontend/apps/web, frontend/apps/mobile (useCastSession) ; not frontend/apps/tv
server-driven receiver: docs/adr/0025-server-driven-cast.md
labels: i18n player.streamInfo.reason.client_line_exceeded, abr_floor_requested
-->
