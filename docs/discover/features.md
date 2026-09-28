# What you get

A tour of Evey from the side of the people who use it every evening. Each
section ends with where to go when you want the settings behind it.

## A library that looks after itself

Drop files into the folders Evey watches and they appear on their own, usually
within seconds: the server notices new files, recognises films and episodes
from their names, and fetches posters, backdrops, summaries, cast, crew and
ratings in the language you chose. Films of the same saga are grouped into
collections, series into seasons, music into artists and albums, photos into
albums.

When a name is too ambiguous to recognise, the title is kept rather than
lost, and an administrator can match it by hand or edit its details. What you
change by hand stays changed: the next refresh does not overwrite it.

Four kinds of library: **films**, **series**, **music** and **photos**.

Settings: [First steps](../guide/first-steps.md#add-your-libraries) ·
[Administration](../guide/admin-settings.md#media).

## Playback that just starts

You press Play; Evey decides how. If the device can read the file as it is, the
file is sent untouched, in its original quality. If only the container is the
problem, it is repackaged on the fly without touching the picture. If the
device really cannot decode it - a codec it does not know, HDR on a screen
that cannot show it, a connection too thin for the original - Evey converts
only what is needed, with the graphics card when the server has one.

While you watch:

- **resume anywhere** - stop on the TV, pick up on the phone at the same
  second;
- **skip intro, recap and credits** with one button, from community databases
  or from the server's own detection;
- **next episode** starts on its own, with a countdown you can cancel;
- **timeline thumbnails** show where you are going when you drag the bar;
- **audio and subtitle tracks** follow your language preferences, can be
  changed mid-film without starting over, and subtitles can be restyled,
  shifted in time or added from a file;
- **chapters**, a **sleep timer**, playback **speed**, and keyboard shortcuts
  on the web.

Settings: [Viewer settings](../guide/viewer-settings.md#in-the-player) ·
[Transcoding and hardware](../transcoding.md).

## Something for everyone at home

Each account holds several **profiles**, each with its own watch history,
resume points, language, favourites and home page. The profile picker opens
the app, like on a family TV.

- **Kids profiles** open without friction; leaving them for an adult profile
  asks for a four-digit PIN.
- **Age limits** are set per profile and enforced by the server: a title above
  the limit is not hidden by the app, it is never sent. If the server cannot
  read a profile's limit, it shows nothing rather than everything.
- **Invitations**: an administrator sends a link, the recipient picks a
  password, and arrives with the libraries and the number of profiles decided
  for them.
- **Requests**: a user can search for a film, a series or an artist that is
  missing and ask for it; the administrator approves, declines or postpones
  with a note.

Settings: [Viewer settings](../guide/viewer-settings.md#profiles-and-parental-control) ·
[Administration](../guide/admin-settings.md#users-and-access).

## Watching together, from different homes

**Watch Together** opens a room around a film, an episode or a music queue.
Share the six-character code or the link; everyone who joins plays in step
with the host - pause, seek and resume included. Someone who fell behind can
catch up on their own without disturbing the room.

## Music and photos

Music has artists, albums, playlists, shuffle, a queue, gapless playback for
albums that need it, and a mini-player that keeps going while you browse.
Photos have albums, a full-screen viewer with the picture's shooting details,
and a slideshow.

## Your own shelves

- **Favourites** and **ratings** per profile.
- **Collections**: the sagas Evey builds itself, plus your own, reorderable.
- **Smart collections**: a saved filter (genre, year, resolution, language...)
  that fills itself as the library grows.
- **Playlists** for films, episodes and tracks, playable in order or shuffled.
- **History** and **statistics** of what you watched, which you can clear.
- A **home page** you can reorder: continue watching, up next, recently
  added, recommendations, collections.

## Offline, on the phone

The Android app downloads a film, a season, a whole series, a collection or a
playlist for the plane. When a film is too large for the phone, it offers a
smaller copy prepared by the server. Downloads respect the profile that made
them: a kids profile cannot open a film another profile downloaded above its
age limit.

## Keeping an eye on the server

Administrators get a live view of who is watching what and how - played as is,
repackaged or converted, and why - with the server's load, its measured
conversion capacity, streaming quality per device, and a media health page that
finds and repairs files that seek badly. The database is backed up before
every update, and on a schedule if you set one.

Settings: [Administration](../guide/admin-settings.md#monitoring-and-maintenance).

## Extending it

Plugins add metadata sources, notifications, webhooks, home-page widgets and
themes. They run in a sandbox with only the permissions they declare, and an
administrator sees those permissions before installing. A theme is a
stylesheet: it restyles the interface without touching its code.

Read more: [Plugins](../plugins.md) · [Theme API reference](../themes/api-reference.md).

## Try it

The public demo at [demo.eveyproject.org](https://demo.eveyproject.org) is the same server you would
install, with a small library of freely licensed films. Sign in with `demo` /
`demo`. Browse, play, make collections, open the administration: settings are
shown but most of them are locked, and the instance is reset regularly.
