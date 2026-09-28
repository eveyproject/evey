# First steps

The container is running and you have opened `http://<your-ip>:9091`. This
page walks through the first ten minutes: the setup wizard, the libraries, the
people, and the three things worth checking before you tell anyone the
address. If the server is not running yet, start with
[Installation](../installation.md).

## The setup wizard

A server with no administrator opens on a four-step wizard.

1. **Language.** The default language for new profiles and for metadata
   (titles, summaries, genres). You can change it later in
   **Admin → Server Config → Language & Region**, along with the region that
   decides which age ratings apply.
2. **Welcome.**
3. **Administrator account.** A username of at least three characters and a
   password of at least eight. The optional **server name** ("Living room")
   appears on the sign-in page, in the apps' server list and in e-mails.
4. **First library.** A name and a type. You can skip the details here and
   finish in the admin panel.

## Add your libraries

**Admin → Libraries → Add Library.** A library has a name, a type (films,
series, music or photos) and one or more folders.

**The folders are paths inside the container**, not on your host. With the
default compose file, your media folder is mounted at `/media`, so a library of
films in `/srv/media/films` on the host is `/media/films` for Evey. A second
mounted folder (`/media2`, ...) is added the same way. The picker lists the
folders it can see.

Keep films and series in separate libraries: the type tells the scanner what
to look for. Evey recognises names like these:

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

The year helps a lot for films. For episodes, `S01E05` or `1x05` is what
matters. Release tags (resolution, source, codec, audio, language tags such
as `VOSTFR` or `MULTI`, edition names such as "Director's Cut") are stripped
before the search. If your files carry a tag the scanner does not know, add it
in **Admin → Server Config → Parser tokens**.

Per library, you also choose:

| Option | What it does |
| --- | --- |
| **Realtime monitoring** | New, renamed and deleted files are picked up as soon as they appear. A periodic check (every 2 minutes by default) catches what the file system did not report, such as changes on a network share. |
| **Scan on startup** | Rescans the library each time the server starts. |
| **Schedule** | A full rescan every 6 or 12 hours, daily, weekly, or on your own cron expression. |

The first scan starts on its own. Posters and details arrive in the
background, so a large library fills in over a few minutes.

**When a title is wrong or missing.** In the library list, **Match orphans
only** retries the titles that were never recognised, and **Re-match all**
redoes the search for every title (slow, but it corrects old mistakes after
a big renaming). On a single title, **Edit Metadata** lets you fix the title
or paste the right TMDB or IMDb id.

## Metadata sources

Evey ships with a key for **TMDB**, the source of titles, posters, cast and
collections, so metadata works out of the box. Other sources are optional and
their keys go in **Admin → Server Config → API Keys**:

| Key | Adds |
| --- | --- |
| TMDB (your own) | Your own quota instead of the shared one. |
| OMDb | A second metadata source, to complete a title TMDB knows little about. |
| OpenSubtitles | Searching and downloading subtitles a file does not carry. |
| TheIntroDB | More intro, recap and credits timings (500 lookups a day instead of 100). |

A key set in the panel wins over the same key given in the environment.

## Invite the household

**Admin → Users** has two ways in:

- **Create User**: you choose the username and the password.
- **Create Invite Link**: the person chooses their own. A link can expire, be
  limited to a number of uses, give access to some libraries only, cap the
  number of profiles the new account may create, and even create an
  administrator.

For each user you can later restrict the **libraries** they see, cap their
**streaming bitrate**, limit **profiles** and **devices per profile**, see
their history, or **connect as** them to see exactly what they see.

Profiles, kids profiles and the parental PIN are set by each account itself:
see [Viewer settings](viewer-settings.md#profiles-and-parental-control).

## Let the server measure itself

The first time it starts on a machine, and again when the hardware changes,
Evey measures how many videos it can convert at once, with the processor and
with the graphics card if there is one. It takes about two minutes, runs only
when nobody is watching, and a player asking for a conversion meanwhile is
told to try again shortly. The result is in **Admin → Transcoding**: that
number is the ceiling you can lower, never raise. To use a graphics card, see
[Transcoding and hardware](../transcoding.md).

## Before you share the address

1. **Remote access.** Evey opens itself to the internet only when you gave it
   a public address. Check **Admin → Remote Access**: if the server is
   reachable from outside but remote access is off, the page says so in red,
   and everyone outside your network gets an error. See
   [Administration](admin-settings.md#remote-access).
2. **HTTPS.** Evey speaks plain HTTP. Put it behind a reverse proxy before
   publishing it: [Reverse proxy](../reverse-proxy.md).
3. **Backups.** The server backs its database up before every update. For
   regular backups, set a schedule: [Backups](../backups.md). Keep
   `./evey-data` safe - it holds the database and the key that signs every
   session.

## Install the apps

Open the Android or Android TV app, type the server's address, and sign in.
On a TV, choose **Quick Connect**: the TV shows a code and a QR code, and you
approve it from **Settings → Quick Connect** on a phone or a computer that is
already signed in. More in [On every screen](../discover/apps.md).
