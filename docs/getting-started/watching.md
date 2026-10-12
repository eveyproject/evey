# Watching with Evey

This page is for the people who watch: family, friends. Someone else runs
the Evey server; you just want to sit down and press play.

## Getting in

The person who runs Evey gives you two things:

- **An address**, for example `http://192.168.1.20:9091`.
- **An account**: a username and a password, or an invitation link. With the
  link, you choose your own username and password.

Then:

- **On a computer**, open the address in your browser and sign in.
- **On an Android phone, tablet or TV**, open the Evey app. Type the address
  on the first screen, then sign in. Step by step:
  [Connect the apps](apps.md#the-server-address-to-enter).
- **On a TV**, choose **Quick Connect** to avoid typing your password with
  the remote. The TV shows a code; you approve it from your phone or
  computer:
  [Quick Connect](apps.md#quick-connect-signing-in-a-tv-without-typing-a-password).

## Choosing a profile

After signing in, **Who's watching?** shows the profiles of the account.
Pick yours. Each person has their own profile, which remembers where they
stopped, what they watched, their favourites and their languages.

A profile marked **Kids** only shows titles suited to the child's age.
Leaving it for another profile asks for that profile's 4-digit PIN. More
about this: [Profiles, kids profiles and the PIN](../administration/users.md#profiles-kids-profiles-and-the-pin).

## Finding something

- **Home** starts with what you left halfway, ready to resume. Then come rows
  such as **Recently Added**, and one row per library. **Customize** chooses
  which rows you see.
- **Libraries** and **Genres** let you browse everything.
- **Search** finds a film, a series or a song by its name.
- **Add to Favorites** on a title keeps it under **Favorites**. **Watch
  History** lists what you have watched on this profile.

## Playing

- On a title's page, **Play** starts it. **Resume** continues where you
  stopped, even if that was on another device. **Play from the beginning**
  starts over.
- In the player, the settings menu lets you choose the **Audio** language and
  the **Subtitles**. To get your languages chosen for you every time, set them
  in **Settings → Playback → Default Audio Language** and **Default Subtitle
  Language**.
- Some subtitles take a moment the first time anyone picks them. The player
  then shows **Subtitles are being prepared…**; the video keeps playing, and
  the subtitles appear by themselves once ready. More:
  [Subtitles](../playback/subtitles.md).
- In a series, **Skip Intro** appears during the opening when Evey has found
  it, and **Up Next** offers the next episode at the end.

You never choose a quality: Evey adapts the video to your device by itself.

## Asking for a title

Something missing? Go to **Requests**:

1. Search for the film, series or artist.
2. Press **Request**. The administrators are told.
3. Follow it in **My Requests**: **Pending**, **Approved**, **Rejected** or
   **Later**, with the note they left. You get a notification when it
   changes.

A title already on the server is marked **Already available**, with **Watch
now**. If the screen says **Requests are disabled**, the administrator has
turned requests off for your account.

## On the phone, the TV and the Chromecast

- **Downloads** (Android app): keep a film on the phone to watch without a
  connection, for 30 days:
  [What each app adds](apps.md#what-each-app-adds).
- **Cast**: send the picture to a Chromecast or a TV with Chromecast built
  in: [Casting](../playback/casting.md).
- **Play on**, in the web and Android players: move what you are watching to
  another of your Evey devices that is open, and carry on there.
- **Watch Together**: several people watch the same title at the same
  moment, each on their own screen. Friends without an account can join with
  a code: [Watching together](../playback/watching-together.md).

## When something does not work

Note three things: the time, the title, and the device. Then tell the person
who runs Evey.

If a film looks worse than usual or is slow to start, open **Stream info**
in the player (the (i) button in the web player). It shows whether the file
is sent as it is or adapted for your device, and why. That helps the person
who runs Evey: [Reading Stream info](../playback/how-playback-is-decided.md#reading-stream-info).

For them: [Troubleshooting](../operations/troubleshooting.md) lists the usual
causes, and [Reporting a problem](../operations/troubleshooting.md#reporting-a-problem)
says what to put in a report.

<!-- sources
connect and sign-in: docs/getting-started/apps.md ; invitation links: docs/administration/users.md ; quick connect: frontend/apps/tv/app/quick-connect.tsx
profile picker, kids badge, PIN: i18n auth.profiles.select.title, kidsBadge, pinPromptTitle, pinInvalid (4 digits) ; docs/administration/users.md (Profiles, kids profiles and the PIN)
navigation: i18n common.nav.home, libraries, genres, search, favorites, requests, watchHistory ; frontend/apps/web/src/components/layout/Sidebar.tsx
home rows: frontend/apps/web/src/routes/_auth/home.lazy.tsx (resume row, recently added, library sliders, Customize) ; i18n common.feeds.labels.resume_playback, media.home.recentlyAdded, media.home.customize ; search: frontend/apps/web/src/routes/_auth/search.lazy.tsx (media + music tracks)
title page: i18n media.play, media.resume, media.playFromStart, media.addToFavorites
player: i18n player.audio, player.subtitles, player.skipIntro, player.nextEpisodeOverlay.upNext, player.streamInfo.* ; profile languages: i18n auth.settings.playback.audioLanguage, subtitleLanguage
requests: frontend/apps/web/src/routes/_auth/requests.lazy.tsx ; frontend/apps/mobile/app/(auth)/(tabs)/requests.tsx ; frontend/apps/tv/app/(auth)/(tabs)/requests.tsx ; i18n requests.* (request, tabMine, statusPending, statusApproved, statusRejected, statusLater, adminNote, inLibrary, watchNow, disabledTitle) ; notification common.requestAnswered
downloads, cast, play on: docs/getting-started/apps.md ; i18n player.cast, player.playOn.* ; watch together: i18n player.watchTogether
administrators told of a request: internal/app/notification/event_handlers.go - handleMediaRequestCreated
subtitles being prepared: i18n player.subtitlesPreparing ; docs/playback/subtitles.md
guests join with a code: docs/playback/watching-together.md ; i18n auth.login.guestJoinTrigger
-->
