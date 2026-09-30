# First setup

The server is running and you opened `http://<server-address>:9091`. Four
steps remain before people can watch: the setup wizard, the libraries, the
users, and the apps.

## 1. The setup wizard

A server with no administrator opens on a wizard of four steps:

1. **Language**: the default language of new profiles and of metadata (titles,
   summaries, genres).
2. **Welcome**.
3. **Administrator account**: a username of at least 3 characters and a
   password of at least 8. The optional server name appears on the sign-in page
   and in the apps' server list.
4. **First library**: a name and a type. You can skip it and add libraries
   afterwards.

<figure class="evey-shot" markdown="span">
  ![The admin dashboard of the demo server](../images/admin-dashboard.webp){ loading=lazy }
  <figcaption>The administration once signed in: the dashboard, and every section in the side menu.</figcaption>
</figure>

## 2. Add your libraries

**Admin → Libraries → Add Library**: a name, a type (Movies, Shows, Music or
Photos) and one or more folders.

The folders are paths **inside the container**. With the compose file from the
installation, your media folder is `/media`: a film folder at
`/srv/media/films` on the host is `/media/films` for Evey.

The first scan starts on its own. New files are picked up as they appear, and
posters, summaries and cast arrive in the background, so a large library fills
in over a few minutes.

How files should be named, how scans work and how to correct a wrong match:
[Libraries and metadata](../administration/libraries.md).

## 3. Add the people

**Admin → Users** creates an account with a password you choose, or an
invitation link with which the person sets their own. Each account then creates
its profiles: one per person in the household, kids profiles included.

What an administrator can restrict per account, and how profiles and parental
control work: [Users, profiles and parental control](../administration/users.md).

## 4. Connect the apps

The web app is the address you already opened. The Android and Android TV apps
ask for the server address, then a sign-in; a TV can be signed in with a code
approved from a phone. See [Apps and devices](apps.md).

## What the server does by itself

None of this needs a setting:

- **Playback.** Each device reports what it can decode, and the server chooses,
  title by title, between sending the file as it is, repackaging it, or
  re-encoding only what the device cannot play. See
  [How playback is decided](../playback/how-playback-is-decided.md).
- **Capacity.** On its first quiet moment the server measures how many videos it
  can transcode at once, with the processor and with any graphics card mapped
  into the container, and never admits more. The measurement takes a few
  minutes; a playback that needs a transcode meanwhile is asked to wait. See
  [Transcoding and server capacity](../playback/transcoding.md).
- **Remote access.** It opens when the server was given a public address, and
  stays closed otherwise. See
  [Remote access and HTTPS](../administration/remote-access.md).
- **Backups.** The database is dumped before every update that changes it. A
  regular schedule is one setting away: [Backups and updates](../backups.md).

## Before you share the address outside your home

1. Put the server behind HTTPS: Evey serves plain HTTP.
   [Remote access and HTTPS](../administration/remote-access.md).
2. Set a backup schedule, and keep a copy of `./evey-data`: it holds the
   database and the key that signs every session.
   [Backups and updates](../backups.md).

<!-- sources
wizard: frontend/apps/web/src/routes/setup.tsx ; i18n auth.setup.usernameMin, auth.setup.passwordMin
library types: i18n admin.libraries types (movies, shows, music, photos)
calibration at quiet moment, wait: internal/app/transcoding/calibration_service.go
remote access inference: docs/adr/0026-remote-access-inferred-from-a-published-url.md
pre-migration dump: internal/adapters/driven/postgres/migrate.go
-->
