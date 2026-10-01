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

## Browsers without a Cast session of their own

In the web player the **Cast** button appears when the browser can cast:
Chrome on a computer, which opens its Google Cast picker, and browsers that
support remote playback, such as Safari or Chrome on Android. Firefox shows
none.

Where the browser cannot open a Cast session itself (Safari, Chrome on
Android), a tap goes through the server, which tries in this order and takes
the first device it finds at each step:

1. a Chromecast on the network of the device you are holding, played through
   Evey's receiver as above;
2. a DLNA TV on the server's network. It receives the stream the browser was
   playing (the original file in Direct Play, the transcoded stream
   otherwise): no decision is made for the TV itself;
3. your own Evey apps open and signed in to the same account: a single one
   takes the film over, as with **Play on**; with several, the player points
   to **Play on** to choose;
4. the browser's own device picker.

## When a cast does not work

[A cast froze, went silent, or the phone keeps spinning](../runbooks/incident-runbook.md#8-a-cast-froze-went-silent-or-the-phone-keeps-spinning).

<!-- sources
receiver profile order: frontend/packages/media-caps/src/castUrl.ts - castProfileFor, profileFromReceiverCaps, pickCastProfile, FLOOR_PROFILE
receiver probe: frontend/apps/web/src/cast-receiver/receiverLogic.ts - probeReceiverCaps
no wasm on the receiver: frontend/apps/web/src/cast-receiver/main.ts - CAST_WASM_ENABLED
line and floor: internal/app/transcoding/resolver_line.go
senders: frontend/apps/web, frontend/apps/mobile (useCastSession) ; not frontend/apps/tv
server-driven receiver: docs/adr/0025-server-driven-cast.md
web button and fallbacks: frontend/apps/web/src/features/playback/HLSPlayer.tsx - castAvailable, handleMobileCast ; hooks/hlsPlayerUtils.ts - pickCastTapPath, planRendererLoad, pickPlayOnFallback ; hooks/useCasting.ts - startCast (castDevices[0], renderers[0])
DLNA: internal/app/dlna/service.go (SSDP AVTransport) ; not in frontend/apps/mobile
labels: i18n player.streamInfo.reason.client_line_exceeded, abr_floor_requested
-->
