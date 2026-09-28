# Viewer settings

What every account can change for itself, without asking the administrator.
Most of it lives in **Settings** (the avatar menu, then Settings); the rest is
in the player. The Android and TV apps carry the same settings under their own
Settings screen.

Settings attach to the **profile** in front of the screen, not to the device:
choose English audio on the TV and the phone will use it too.

## Appearance and language

| Setting | What it does |
| --- | --- |
| **Theme** | The colour theme, including those installed by the administrator. |
| **Language** | The language of the interface. Six are available: English, French, German, Spanish, Hindi and Chinese. |
| **Region** | Which country's age ratings are shown and applied. **Automatic** follows the server. |
| **Preferred audio language** | The track chosen first when a file has several languages. |
| **Preferred subtitle language** | The subtitle track shown when one matches; **No preference** leaves them off unless the file forces them. |

Titles and summaries follow the server's metadata language, chosen by the
administrator.

## Playback

| Setting | Default | What it does |
| --- | --- | --- |
| **Auto-play next episode** | on | Starts the next episode when one ends. The Next episode button stays either way. |
| **Max streaming bitrate** | Unlimited | A ceiling for your streams. Lower it on a slow connection: the server converts to fit instead of letting the player stall. |
| **Max cellular bitrate** (phone) | - | A lower ceiling applied only on mobile data. |
| **Preferred video codec** / **audio codec** | Auto | Leave on Auto unless a device misbehaves with one codec: Auto lets the server pick what the device decodes best. |
| **Max audio channels** | Auto | Stereo, 5.1 or 7.1. Choose Stereo if you watch on headphones or TV speakers and dialogue is too quiet in surround mixes. |
| **Subtitle burn-in** | Auto | Some subtitle formats (image-based, or heavily styled) cannot be drawn by every player; Auto burns them into the picture only then. **Always** and **Never** force it, at the cost of a conversion or of missing subtitles. |
| **Allow direct play** / **direct stream** / **transcoding** | all on | The three ways a film can reach you: as is, repackaged, or converted. Switching one off is a troubleshooting tool, not an everyday setting. |

The administrator can set a lower ceiling on your account or on connections
from outside the home; the lowest ceiling always wins.

## In the player

The gear icon opens the player's settings:

- **Quality**: **Auto** adapts to your connection; **Original** asks for the
  file untouched when the device can read it; a fixed height (1080p, 720p...)
  forces a conversion at that size.
- **Audio** and **Subtitles**: switch track at any moment, without restarting
  from the beginning. **Upload subtitle...** adds a subtitle file from your
  computer for this title.
- **Audio sync**: shifts the sound against the picture, in milliseconds
  (positive = sound later). Available when the film is being converted; it
  restarts the stream at the current position.
- **Subtitle sync**: shifts the subtitles (positive = later), instantly.
- **Subtitle style**: size, text colour, background, font, position, shadow.
- **Speed** and **Sleep timer** (a number of minutes, or the end of the
  episode).

Around the timeline: **chapters**, **skip intro**, **skip recap**, **skip
credits** buttons when the server knows where they are, a countdown to the
next episode, **Cast**, **Play on** (send to another Evey app),
**Watch Together** and **Download** (phone).

**Stream info** tells you how the film reaches you - direct play, remux or
transcode - and why. It is the first thing to look at when something plays
badly: "Audio codec this player cannot decode" or "Bitrate above the allowed
limit" usually explains it.

On a computer, `?` shows the keyboard shortcuts: `Space` or `K` to pause,
`J`/`L` or the left and right arrows to seek, up and down for the volume, `F`
for full screen, `M` to mute, `C` for subtitles, `<` and `>` for the speed,
`P` for picture-in-picture, and `0` to `9` to jump to 0-90 % of the film.

## Profiles and parental control

**Settings → Profiles** creates, edits and deletes profiles (the default
profile cannot be deleted). Each profile has:

| Field | What it does |
| --- | --- |
| **Name** and **avatar** | Shown in the "Who's watching?" picker. |
| **Kids profile** | Opens without a PIN; leaving it for another profile asks for the account's PIN. |
| **Max age rating** | Titles above it are never sent to this profile - not in lists, not in search, not to the player. Set by the server, so no app can bypass it. |
| **Language** and **Region** | Override the interface language and the ratings country for this profile. |

**Settings → Parental** sets the account's **4-digit PIN**. The first time,
it asks for the account password; changing it later asks for the current PIN.
A PIN is required as soon as a kids profile exists.

If the server cannot read a profile's age settings, it shows nothing rather
than everything: a broken setting never opens the library to a child.

## Downloads (phone)

**Download** on a film, a season, a series, a collection or a playlist keeps
it on the phone. When a film is too large for the free space, the app offers a
smaller copy prepared by the server, with the expected size of each choice.
**Downloads** lists what is stored and how much space it takes.

A download stays tied to the profile rules: a title above a profile's age
limit is not playable from that profile, even if another profile downloaded
it. A badge says when a download has expired and must be fetched again.

## Notifications

**Settings → Notifications** switches push notifications on for this device
and sets, for films, series and music separately, how much you want to hear:
**All**, **Relevant**, **Important only** or **Disabled**. Push requires the
administrator to have set it up ([Notifications](../notifications.md)).

## Account and devices

- **Account** (profile card): username, e-mail, password (eight characters
  at least) and avatar. The e-mail is what lets you reset a forgotten
  password, when the server can send mail.
- **Devices**: the devices registered for push notifications, which you can
  remove.
- **Quick Connect**: type, or scan, the code shown by a TV or another device
  to sign it into your account without typing your password on it.
- **Watch history** (profile menu): everything you watched, by day, with a
  button to remove an entry or clear it all.
- **Plugins**: when the administrator installed plugins with their own
  preferences, they appear here.

## Requests

If the administrator allows it, **Media Requests** lets you search for a film, a
series or an artist that is not in the library and ask for it. **My
Requests** shows the answer: approved, declined or later, with the
administrator's note.
