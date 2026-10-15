# Common questions

Short answers to the questions people ask the person who runs Evey at home.
Each one says what to do first, then links to the page that explains why, for
the day you want to understand.

## Why does the film take a while to start?

Usually because the server is adapting it. Your TV or phone cannot play that
file exactly as it is stored, so the server converts it as you watch, and the
first seconds go into preparing that. A film that starts almost at once is
being sent as it is.

Open **Stream info** in the player (the (i) button) and it tells you which of
the two is happening, track by track, and why. If one title is slow every
time while the others are fine, the file itself is probably the cause, and
the person who runs Evey can check it in **Admin → Media Health**.

More: [How playback is decided](../playback/how-playback-is-decided.md#reading-stream-info)
and, for the person who runs it, [Media health](../runbooks/media-health.md).

## Why is the picture less sharp on my phone when I am away?

Because a ceiling applies to viewers outside the home. The person who runs
Evey can set two limits in **Admin → Transcoding**: one for everyone, and a
lower one for viewers outside the local network, so that a film keeps playing
on a mobile connection. Each account can also carry its own **Max Bitrate**. A
title above the limit is converted down to it, and some detail goes with it.

There is nothing to choose on your side: Evey never asks you to pick a
quality. **Stream info** shows which limit applied.

More: [What is encoded](../playback/transcoding.md#what-is-encoded) and
[What an administrator can restrict per account](../administration/users.md#what-an-administrator-can-restrict-per-account).

## Why is the sound in the wrong language?

A track you pick by hand, in the player's settings menu, always wins for that
title. Otherwise the players take the language set in **Settings → Playback →
Default Audio Language**, then the track the file marks as default, then the
first one. Set your language there once and every title follows it. The same
goes for subtitles, with **Default Subtitle Language**.

More: [Which track plays](../playback/audio.md#which-track-plays).

## Does it work on my television?

Yes, in one of three ways:

- **An Android TV or Google TV** runs the Evey app, made for the remote. On
  the sign-in screen, **Quick Connect** shows a code that you approve from
  your phone or computer, so you never type a password with the remote.
- **A TV with a web browser** opens the same kind of screen at the server's
  address followed by `/tv`.
- **A Chromecast**, or a TV with Chromecast built in, receives the picture
  from the Cast button in the web or Android player.

More: [Connect the apps](apps.md#quick-connect-signing-in-a-tv-without-typing-a-password)
and [Casting](../playback/casting.md).

## Can I watch in the train, without a connection?

With the Android app, yes. On a film or episode, the Android app can keep a copy on
the phone (**Downloads**), and it stays playable for 30 days without contacting the server. When
the person who runs Evey turns it on, the app also offers a smaller copy for a
film too large for the phone. A kids profile cannot open a download that is
above its age limit, even if another profile made it.

More: [What each app adds](apps.md#what-each-app-adds).

## Can I watch a film with a friend who lives elsewhere?

Yes. Start the film in the web player or the Android app and press **Watch
Together**: you get a six-character code. Your friend enters it, from their
own account or, without an account, with **Watch together** on the sign-in
page. Each person sees the same moment, on their own screen, in the version
that suits their own device.

More: [Watching together](../playback/watching-together.md).

## How do I give a friend access without opening the whole catalogue?

In **Admin → Users**, create an invitation link and limit it to the libraries
that person may see: the others will not appear for them anywhere, not even in
search. You can also cap the quality they receive (**Max Bitrate**) and how
many screens they may play at once. For a single film, a **Public download
link** on its page lets anyone with the link download the file for 48 hours,
without an account.

More: [Create an account](../administration/users.md#create-an-account) and
[Public download links](../administration/users.md#public-download-links).

## Why does a film have no poster, or the wrong one?

Because the file name did not point to the right title. For a film, the year
matters most (`The Mirror (1999)`); for an episode, the season and episode
number (`S01E05`). To fix one title, open it, choose **Edit Metadata**, then
**Identify on TMDB** and pick the right match. A new, renamed or deleted file
shows up by itself within a few minutes; **Scan** on the library's row forces
it.

More: [Name files so they are recognised](../administration/libraries.md#name-files-so-they-are-recognised)
and [Fix a wrong or missing title](../administration/libraries.md#fix-a-wrong-or-missing-title).

## The subtitles say they are being prepared. Is something wrong?

No. The first time anyone picks some subtitles, the server extracts them from
the file; the video keeps playing and they appear on their own once ready.
It happens once per track. If the message stays for several minutes, the file
has no index for that track and the server is reading it through, once.

More: [The first time a subtitle is shown](../playback/subtitles.md#the-first-time-a-subtitle-is-shown).

## Everything is slow for everyone at the same time. Why?

The server converts a limited number of videos at once, a number it measured
on its own hardware. When that room runs short, it first lowers the quality of
a new stream, then moves it to the processor, and only then shows **The
server is busy transcoding other streams**. Films sent as they are do not
count, which is why a catalogue that suits the devices at home rarely hits
the limit. The person who runs Evey sees how busy it is in **Admin → System
Health**.

More: [Sharing the capacity](../playback/transcoding.md#sharing-the-capacity).

## Do my films or my viewing history leave the house?

No. The apps talk to your Evey server and to nothing the project operates.
What you browse, play and request stays on that server, and who can read it
is up to the person who runs it. The exceptions are the ones you would
expect: a poster fetched from its public source, a Chromecast that loads the
receiver page, a link you open yourself.

More: [Privacy policy](../privacy.md).

## How do I update without breaking anything?

For the person who runs it, in the folder that holds the compose file:

```bash
docker compose pull && docker compose up -d
```

That is the whole update. When the new version needs to change the database,
it saves a copy first, in `./evey-data/backups/`. Evey says in the admin panel
when a newer version exists; it never updates itself.

More: [Updating Evey](../backups.md#updating-evey).

## I forgot my password

If the sign-in page shows **Forgot your password?**, use it: a reset link is
e-mailed to you. That link is there only when the person who runs Evey has
turned on **Password recovery** and set up e-mail. Otherwise, ask them: in
**Admin → Users**, **Edit User** has a **New password** field. As a last
resort, when the administrator's own password is lost, `evey admin
reset-password` prints a new one on the server.

More: [E-mail and notifications](../notifications.md) and
[Give a user a new password](../runbooks/recovery.md#give-a-user-a-new-password).

<!-- sources
stream info and the three outcomes: docs/playback/how-playback-is-decided.md ; i18n player.streamInfo.*
remote ceiling: i18n admin.calibration.maxBitrateLabel, maxBitrateRemoteLabel ; per-account bitrate: internal/app/user/bitrate_limit.go
track choice: internal/app/transcoding/decision.go - selectedAudioSpec ; i18n auth.settings.playback.audioLanguage, subtitleLanguage
tv: frontend/apps/tv/app/quick-connect.tsx ; /tv page: docs/getting-started/apps.md ; cast: docs/playback/casting.md
downloads: frontend/apps/mobile/src/lib/offlineAccess.ts - OFFLINE_DOWNLOAD_TTL_MS ; cmd/evey/http.go (downloads.quality_choice)
watch together: docs/playback/watching-together.md
invites and restrictions: frontend/apps/web/src/features/admin/CreateInviteForm.tsx ; internal/app/user/access_service.go ; share links: internal/adapters/driving/grpcapi/share_handler.go - durationHours = 48
metadata: docs/administration/libraries.md ; i18n matchOrphans, refreshMetadata
subtitles: docs/playback/subtitles.md
capacity: internal/app/transcoding/video_session_limiter.go ; i18n errors.transcode.max_concurrent_reached
privacy: docs/privacy.md
update: docs/backups.md ; internal/adapters/driven/postgres/migrate_backup.go
password: frontend/apps/web/src/routes/login.tsx (passwordRecoveryEnabled) ; i18n admin.serverSettings.passwordRecovery, admin.users.newPassword ; cmd/evey (admin reset-password)
-->
