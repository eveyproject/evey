# Privacy policy

Last updated: 2026-10-04.

This policy covers the Evey apps for Android and Android TV.

In short: the Evey project runs no service that receives your data. There is
no account with us, no analytics, no advertising, and no crash reporting to
us. The apps talk to the Evey server that you, or the person who invited you,
installed and runs.

## What the apps send, and to whom

**To your Evey server, and to nothing else we operate.** The apps send what a
media player needs to work:

- your username and password when you sign in (or the Quick Connect code),
  then a session token, a key that keeps you signed in;
- what you browse, search, play, rate, favourite or request, and your playback
  position, so you can resume on another device;
- the name of the device (as Android reports it), so you can tell your devices
  apart in **Play On** and in the server's list of sessions;
- technical playback reports: which audio and video formats the device can
  decode, and errors met while playing, so the server can pick a stream the
  device can play;
- a picture, only when you choose one as a profile avatar, and files you pick
  yourself (an external subtitle, a plugin, a backup) as an administrator.

Who can read this data is up to the operator of that server. The Evey project
has no access to it.

**To third parties, only in these cases:**

- **Posters and album art.** Some pictures shown in media requests and music
  come straight from their public source (The Movie Database, the Cover Art
  Archive, IMDb). The device fetches them like a web browser would, which
  reveals its IP address to that host.
- **Casting.** Casting to a Chromecast goes through Google Cast, part of Google
  Play services on your phone. The Chromecast loads the Evey receiver page from
  `cast.eveyproject.org`, hosted by GitHub Pages. The media itself streams from
  your server.
- **Voice search on Android TV.** The microphone button hands over to the
  television's own voice recognizer (usually Google's). The app receives the
  recognized text, never the recording.
- **Links you open.** A trailer, a The Movie Database or IMDb page, or the
  Play Store page for an update opens in the matching app or browser, under
  that service's own policy.
- **Plugins.** Plugins are installed by the server's administrator. A plugin
  may show content from an outside service (a weather plugin, for instance).

## What stays on the device

The server address, your session token (in Android's encrypted storage), your
preferences, and the films and music you download for offline viewing. Removing
the app removes them. Biometric unlock (fingerprint or face), when you turn it
on, is handled by Android: the app only learns whether it succeeded.

## Permissions

| Permission | Why |
| --- | --- |
| Internet, Wi-Fi state | Reach your server, tell your home network apart from the outside. |
| Camera | Scan a Quick Connect QR code. Nothing is recorded or sent. |
| Notifications, foreground service | Show playback and download progress, keep playing with the screen off. |
| Biometrics | Optional app lock. |

The phone app does not have the microphone permission.

## This website

`eveyproject.org` is hosted by GitHub Pages, which sees the visitor's IP
address. The site serves its own fonts and scripts, so the browser contacts no
other service. It sets no cookie of its own and has no analytics.

## Children

The apps are not aimed at children. Kids profiles are set up by the server's
operator, who holds that data.

## Contact

Questions about this policy: [contact@eveyproject.org](mailto:contact@eveyproject.org).
For data held on an Evey server, contact the person who runs that server.

<!-- sources
no tracker: frontend/apps/mobile/package.json, frontend/apps/tv/package.json
permissions: frontend/apps/mobile/android/app/src/main/AndroidManifest.xml
device name: frontend/apps/mobile/src/hooks/useDeviceRegistration.ts
playback reports: frontend/packages/client/src/native/useAudioCapsBeacon.ts
outside posters: internal/app/mediarequest/service.go - trustedPosterHosts
cover art: frontend/packages/client/src/lib/artwork.ts - coverArtUri
cast receiver: docs/adr/0025-server-driven-cast.md
voice search: frontend/packages/voice-search/android/src/main/java/com/evey/voicesearch/EveyVoiceSearchModule.kt
store page: frontend/apps/tv/src/lib/appUpdate.ts - openStorePage
-->
