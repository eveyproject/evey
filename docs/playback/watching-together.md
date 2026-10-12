# Watching together

Watch the same film at the same moment as other people, each on your own
screen. One person opens a room; the others join it with a code. There is
nothing to set up on the server.

In short:

1. Start the film in the web player or the Android app, and press **Watch
   Together**. You are the host of the room, and you see its code: six
   letters and digits.
2. Send the code to the people you want with you.
3. They join with the code: from their own account (see the table below),
   or, without an account, with **Watch together** on the sign-in page.

Who can pause or move the film for everyone depends on the app: see
[What each member controls](#what-each-member-controls).

Each person still gets the version of the film that suits their own device,
as explained in [How playback is decided](how-playback-is-decided.md).

## Starting and joining a room

| | Web | Android | Android TV |
| --- | --- | --- | --- |
| Start a room | **Watch Together** in the player | **Watch Together** in the player | - |
| Join with an account | The room's link, or the code in **Listen Together** → **Join a room** | The code in **Listen Together** | The code in **Settings** → **Watch Together** |
| Join without an account | **Watch together** on the sign-in page, with the code | Same | - |

Typing the code is forgiving: capitals or not, spaces, and `O` for `0` or
`I` or `L` for `1` are all accepted.

Joining with an account follows the usual rules: the library must be shared
with that account, and the profile's parental settings must allow the title. A
restricted profile cannot open a room on a title it may not watch either.

## What each member controls

| | Host | Other members |
| --- | --- | --- |
| Android | Play, pause and seek move the whole room. | Play, pause and seek stay on your own screen. |
| Android TV | Play and pause move the whole room; a seek stays on the TV. | Same as Android. |
| Web | Play, pause and seek stay on your own screen, for the host too. | Same. |

In the web player, explicit buttons replace the host's control. **Sync
everyone** brings the whole room to your position and resumes playback; any
member can use it, not only the host. **Catch up** brings you back to where
the room is, without moving anyone else; it is offered on the guest page
(joined with a code, without an account), not in the signed-in player.

Someone who joins late starts where the room is now, not where it was when the
last command was sent.

If the host leaves, the member who has been in the room longest becomes the
host. When the last member leaves, the room closes, and the guest access its
code gave is withdrawn. A room nobody ever entered is removed after an hour.

## Guests without an account

A guest who enters a code on the sign-in page watches that one title and
nothing else:

- streaming only, never downloading;
- video rooms only, not music rooms;
- the access lasts while it is used, lapses after three hours without
  playback and after 24 hours in any case, and ends as soon as the room
  closes.

!!! warning
    The code is the only thing that protects a room, so share it only with the
    people you want in it. Wrong codes are limited per address and across the
    whole server, which keeps guessing them out of reach.

## Listening together

A music room is started from **Listen Together** with a queue of albums,
tracks and playlists. The host plays, pauses, and moves through the queue;
every member follows. Guests without an account cannot join music rooms.

<!-- sources
room, code, normalisation: internal/domain/syncplay/model.go - GenerateCode (Crockford base32), NormalizeCode, OldestMember
host-only play/pause/seek, resync, catch_up: internal/app/syncplay/hub.go - HandleCommand, liveStateOf
room cap, janitor: internal/app/syncplay/hub.go - defaultMaxRoomsPerHost, StartJanitor ; cmd/evey/main.go (StartJanitor 10 min, 1 h)
guest token: internal/adapters/driving/http/syncplay_handler.go - syncplayJoinByCodeHandler, guestTokenTTL (sliding 3 h, 24 h cap), joinCodeFailureThreshold ; internal/adapters/driving/http/media_handler.go (guest cannot download)
guest revoked with the room: internal/app/syncplay/hub.go - revokeGuestTokens
access and parental: internal/adapters/driving/grpcapi/syncplay_handler.go - requireMediaAccess
web controls: frontend/apps/web/src/features/syncplay/SyncPlayBar.tsx - onResync, onCatchUp ; frontend/apps/web/src/features/syncplay/useSyncPlay.ts - sendResync, sendCatchUp ; frontend/apps/web/src/routes/_auth/player/$mediaId.lazy.tsx (no sendPlay/sendPause/sendSeek from the video player)
mobile and TV controls: frontend/apps/mobile/app/(auth)/player/[id].tsx (sendPlay, sendPause, sendSeek) ; frontend/apps/tv/app/(auth)/player/[id].tsx (sendPlay, sendPause only) ; frontend/apps/tv/app/(auth)/syncplay/index.tsx (join only) ; frontend/apps/tv/app/(auth)/(tabs)/settings.tsx (Watch together section)
labels: i18n media.syncplay.title, join, joinByCode, resyncAll, catchUp ; auth.login.guestJoinTrigger ; common.nav.syncplay
viewer steps: i18n player.watchTogether, auth.login.guestJoinTrigger
-->
