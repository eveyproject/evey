# Known limitations

What Evey does not do yet, said plainly, so that nobody discovers it the
evening they wanted to watch a film. Each point says what to use instead.

## No app for iPhone, iPad or Apple TV

There is an Evey app for Android phones and tablets, and one for Android TV
and Google TV. There is none for iPhone, iPad or Apple TV. On those devices,
open the server's address in the web browser.

More: [Connect the apps](apps.md).

## The Android apps are still in testing

The Android and Android TV apps are not on Google Play for everyone yet.
Only the accounts invited to the test can install them. The web app and the
`/tv` page in a television's browser need no installation and work for
everyone.

More: [Connect the apps](apps.md).

## Watching without a connection: Android app only

Only the Android app keeps a copy of a film or an episode on the device
(**Downloads**), playable for 30 days without contacting the server. The web
player, the Android TV app and the `/tv` page play from the server and need
the connection all along.

More: [What each app adds](apps.md#what-each-app-adds).

## No notifications on the phone while the app is closed

The Android and Android TV apps show their notifications in their
**Notifications** screen, when they are open. They do not ring on the phone
or the TV while closed. Notifications that reach you outside the app come
from the web browser (once turned on, over `https://`) or by e-mail, when the
person who runs Evey has set it up.

More: [Where notifications appear](../notifications.md#where-notifications-appear).

## Profiles are managed on the web or on the phone

The Android TV app switches between profiles and changes some settings of
the one in use, but it cannot create or delete a profile. Do that in the web
app or the Android app.

More: [Users, profiles and parental control](../administration/users.md#profiles-kids-profiles-and-the-pin).

## Casting: from Chrome or the Android app

The **Cast** button exists in the web player in Chrome and in the Android
app. Firefox has none, and the Android TV app has none because it already is
the TV. Safari, and Chrome on an Android phone, go through the browser's own
remote playback, which works differently.

More: [Casting](../playback/casting.md).

## Evey never updates itself

The server checks whether a newer version exists and says so on the admin
dashboard, but it installs nothing on its own: the person who runs it
updates when it suits them, with two commands. The Android apps show a
notice when they are older than the server.

More: [Updating Evey](../backups.md#updating-evey) and
[Keeping the apps up to date](apps.md#keeping-the-apps-up-to-date).

<!-- sources
no iOS build or distribution: frontend/apps/mobile has no ios/ directory, .github/workflows/ci.yml builds no iOS target ; apps.md platform list
testing status: docs/getting-started/apps.md platform list
offline copies: frontend/apps/mobile/src/lib/offlineAccess.ts - OFFLINE_DOWNLOAD_TTL_MS ; no downloads screen in frontend/apps/tv or frontend/apps/web
no native push: no expo-notifications or firebase messaging in frontend/apps/mobile/package.json or frontend/apps/tv/package.json ; docs/notifications.md "Where notifications appear"
profiles on TV: frontend/apps/tv uses useUserServiceUpdateProfile only, frontend/apps/mobile also uses useUserServiceCreateProfile and useUserServiceDeleteProfile ; frontend/apps/tv/app/profile-select.tsx
casting: docs/playback/casting.md ; frontend/apps/web/src/hooks/useRemotePlayback.ts
updates: docs/backups.md "Updating Evey" ; frontend/apps/mobile/src/components/admin/UpdateBanner.tsx ; frontend/apps/mobile/src/components/UpdateBanner.tsx
-->
