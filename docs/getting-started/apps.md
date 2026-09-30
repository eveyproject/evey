# Apps and devices

Every app talks to your own server. You give it the server's address once,
sign in, and choose a profile. Resume points, watch history and favourites are
the server's, so they follow you from one device to the other.

| App | Where it comes from | Signing in |
| --- | --- | --- |
| **Web** | Served by Evey itself, at the server's address. Nothing to install. | Username and password, or single sign-on if configured. |
| **Android** (phone, tablet) | The Android app. | Server address, then username and password. |
| **Android TV** | The Android TV app. | Server address, then **Quick Connect** (below) or username and password. |
| **Chromecast** | No app: cast from the web app in Chrome or from the Android app. | Uses the session of the device that casts. |

The administration panel is part of the web app, for administrator accounts.

## Quick Connect: signing in a TV without typing a password

On the TV, choose **Quick Connect**. It shows a code and a QR code. On a phone
or a computer where you are already signed in, open **Settings → Quick
Connect**, and type the code or scan the QR code with the Android app. The TV
is signed in to the same account.

## What each app adds

- **Web.** Keyboard shortcuts in the player (`?` lists them) and the
  administration panel.
- **Android.**
  - **Downloads** for offline viewing. When the administrator has turned it
    on (`downloads.quality_choice`, see
    [Playback delivery and downloads](../environment/playback-delivery.md)),
    the app offers a smaller copy prepared by the server for a film too large
    for the phone. A download stays
    playable offline for 30 days without contacting the server, and only for
    profiles allowed to see it: a kids profile cannot open a film downloaded by
    another profile above its age limit.
  - **Casting** to a Chromecast.
  - **Push notifications** for new films, episodes and music, if the server
    has them configured: see [E-mail and notifications](../notifications.md).
- **Android TV.** An interface built for the remote.

**Play on**, in the web and Android players, sends what you are watching to
another Evey app signed in to the same account and currently open, and
playback continues there.

## Which device plays what

Each app reports what its device can decode, and the server decides for each
title whether to send the file as it is or to adapt it. The same film can be
sent untouched to a TV and re-encoded for an older phone. The player's
**Stream info** panel shows the decision and its reasons: see
[How playback is decided](../playback/how-playback-is-decided.md). Casting has
rules of its own: see [Casting](../playback/casting.md).

<!-- sources
quick connect: i18n auth.quickConnect.*, auth.settings.quickConnect
offline window and profile check: frontend/apps/mobile/src/lib/offlineAccess.ts - OFFLINE_DOWNLOAD_TTL_MS
reduced copy (off by default): cmd/evey/http.go (downloads.quality_choice) ; i18n downloads.reduced.*
play on: i18n player.playOn.* (used by frontend/apps/web and frontend/apps/mobile, not frontend/apps/tv)
-->
