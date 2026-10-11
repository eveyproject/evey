# Watching with Evey

For the people of the household who watch, not the one who runs the server:
how to get in, find something, play it the way you like, and ask for what is
missing. Every section ends with the page that goes into detail.

## Getting in

Someone runs the server and gives you two things: its **address** and an
**account**.

- **The address** is what you type in a browser, or on the first screen of the
  Android and Android TV apps, for example `http://192.168.1.20:9091` at home.
  [The server address to enter](apps.md#the-server-address-to-enter) explains
  where it comes from and what each error means.
- **The account** is a username and a password, or an invitation link with
  which you choose your own password.
- **On a TV**, choose **Quick Connect**: the TV shows a code, which you approve
  from your phone or a browser where you are already signed in, and nothing
  has to be typed with the remote:
  [Quick Connect](apps.md#quick-connect-signing-in-a-tv-without-typing-a-password).

## Choosing a profile

After signing in, **Who's watching?** lists the profiles of the account: one
per person, so that each keeps their own resume points, history, favourites
and languages. A profile marked **Kids** only shows titles suited to its age,
and leaving it for another profile asks for that profile's 4-digit PIN.
How profiles and the PIN work:
[Users, profiles and parental control](../administration/users.md#profiles-kids-profiles-and-the-pin).

## Finding something

- **Home** opens with what you left in the middle, ready to pick up, then
  rows such as **Recently Added** and one per library; **Customize** chooses
  which rows appear.
- **Libraries** and **Genres** browse everything; **Search** finds a film, a
  series or a song by name.
- **Add to Favorites** on a title keeps it under **Favorites**; **Watch
  History** lists what this profile has watched.

## Playing

- On a title's page, **Play** starts it, **Resume** continues where you
  stopped, on any device, and **Play from the beginning** starts over.
- In the player, the settings menu chooses the **Audio** track and the
  **Subtitles**. The languages picked first are those of your profile:
  **Settings → Playback → Default Audio Language** and **Default Subtitle
  Language**. More on subtitles, and on why the first display of some of them
  takes a while: [Subtitles](../playback/subtitles.md).
- **Skip Intro** appears during the opening of an episode when the server has
  found it, and **Up Next** offers the following episode at the end.
- When a title looks worse than expected or starts slowly, **Stream info** in
  the player says whether the file is sent as it is or adapted for your
  device, and why: [How playback is decided](../playback/how-playback-is-decided.md#reading-stream-info).

## Asking for a title

**Requests** searches for a film, a series or an artist the libraries do not
have yet; **Request** sends it to the administrators. **My Requests** shows
where each one stands: **Pending**, **Approved**, **Rejected** or **Later**,
with the note they left, and you are notified when it changes. A title already
on the server is marked **Already available**, with **Watch now**. When the
administrator has turned requests off for your account, the screen says
**Requests are disabled**.

## On the phone, the TV and the Chromecast

- **Downloads** (Android app) keep a title on the phone to watch without a
  connection: [What each app adds](apps.md#what-each-app-adds).
- **Cast** sends the picture to a Chromecast: [Casting](../playback/casting.md).
- **Play on**, in the web and Android players, moves what you are watching to
  another of your Evey devices that is open, and playback continues there.
- **Watch Together** keeps several people on the same moment of the same
  title, each on their own screen: [Watching together](../playback/watching-together.md).

## When something does not work

Note the time, the title and the device, and tell whoever runs the server.
[Troubleshooting](../operations/troubleshooting.md) gives them the usual
causes, and [Reporting a problem](../operations/troubleshooting.md#reporting-a-problem)
what to put in a report when it goes further.

<!-- sources
connect and sign-in: docs/getting-started/apps.md ; invitation links: docs/administration/users.md ; quick connect: frontend/apps/tv/app/quick-connect.tsx
profile picker, kids badge, PIN: i18n auth.profiles.select.title, kidsBadge, pinPromptTitle, pinInvalid (4 digits) ; docs/administration/users.md (Profiles, kids profiles and the PIN)
navigation: i18n common.nav.home, libraries, genres, search, favorites, requests, watchHistory ; frontend/apps/web/src/components/layout/Sidebar.tsx
home rows: frontend/apps/web/src/routes/_auth/home.lazy.tsx (resume row, recently added, library sliders, Customize) ; i18n common.feeds.labels.resume_playback, media.home.recentlyAdded, media.home.customize ; search: frontend/apps/web/src/routes/_auth/search.lazy.tsx (media + music tracks)
title page: i18n media.play, media.resume, media.playFromStart, media.addToFavorites
player: i18n player.audio, player.subtitles, player.skipIntro, player.nextEpisodeOverlay.upNext, player.streamInfo.* ; profile languages: i18n auth.settings.playback.audioLanguage, subtitleLanguage
requests: frontend/apps/web/src/routes/_auth/requests.lazy.tsx ; frontend/apps/mobile/app/(auth)/(tabs)/requests.tsx ; frontend/apps/tv/app/(auth)/(tabs)/requests.tsx ; i18n requests.* (request, tabMine, statusPending, statusApproved, statusRejected, statusLater, adminNote, inLibrary, watchNow, disabledTitle) ; notification common.requestAnswered
downloads, cast, play on: docs/getting-started/apps.md ; i18n player.cast, player.playOn.* ; watch together: i18n player.watchTogether
-->
