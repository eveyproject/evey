# Media health (admin → Media health)

The Media health page inspects every MKV/WebM of a library and gives each file
ONE verdict. The point is to actually fix what can be fixed and to stop
re-reading the same warnings forever - not to keep a list of 600 findings
nobody acts on.

## The four verdicts

| Verdict | What it means | What the server does about it |
|---|---|---|
| **Healthy** | Index, audio timing, container and sizes all check out. | Nothing. |
| **Repairable** | A defect the server can fix by itself, safely. | **Repair all** fixes every one of them in the background (see below). |
| **Your call** | A defect that only the source can fix, or a repair with a real trade-off. | Nothing automatic. Acknowledge it, or re-acquire the file. |
| **Info** | A curiosity with no effect on playback. | Nothing. Acknowledge it to clear the view. |

Every row also carries the detail badges behind its verdict. The badges,
grouped by verdict:

**Repairable**

- *Missing* seek index, or an index that is present but unusable (keyed on
  the audio track, referencing tracks the file no longer has, too sparse to
  land a seek near its target). Repair rebuilds the Cues: the file is
  rewritten to a temp copy, fully re-validated (tracks, duration, index), then
  swapped in. The symptom it cures is slow seek and slow resume.
- *Audio +Ns* - an audio track starts late against the video (a mux defect,
  the classic "lips ahead of the sound"). Repair realigns the track in place.
- *Wrong container* - the `.mkv` is really an MP4/MOV. There is no Matroska
  index to build; repair **renames it to `.mp4`** and updates the library
  entry. Nothing is rewritten.
- *Damaged ~size* - a corrupted stretch in the middle of the file that a
  strict rewrite refuses. Repair can **resync**: it skips the damaged span and
  re-synchronises what follows. Those bytes are lost for good, so the Repair
  dialog asks for that explicitly (*Also resync N damaged file(s)*) - unticked,
  damaged files are left alone.

**Your call**

- *Incomplete download - missing tail ~size* - the declared end of the file
  is beyond its real size and the walk confirmed the cut. The missing tail
  cannot be recovered by any tool: the size tells you whether it is a 9 KB
  stub (invisible in playback) or a missing final scene. Re-acquire the source
  for a real fix.
- *Audio -Ns* - an audio track ends N seconds **before the picture does**
  (measured content against content, never against the container's declared
  duration, which is only a muxer's claim). Playback pads the tail with
  silence; the audio is simply not in the file.
- *Coarse seeking* - the index was rebuilt and a hole remains. The detail
  says why: keyframes nobody cued (a second rebuild closes it), a stretch with
  no keyframe at all (only a re-encode makes it seekable), or **a stretch with
  no picture at all** - the video freezes there whatever the index does, and
  the only remedy is the source.

**Info**

- *Trailing bytes* - surplus bytes after the declared end of the Segment
  (an aborted append, a torrent client's journal). The media is complete and
  plays fine; any future rewrite drops them.

## Acknowledging

*Seen, stop flagging* (per file) and *Mark all as seen* (per group) take a
finding out of the *Awaiting* view. The acknowledgement is tied to the file's
identity **and its bytes**: if the file changes (size or mtime), the next scan
diagnoses it afresh and the flag comes back. *Flag again* undoes it. An
acknowledgement posted while a scan is running is kept when that scan
completes (for an unchanged file).

The views at the top of the table (*Awaiting*, *Repairable*, *Your call*,
*Info*, *Seen*, *Backups*, *All*) plus the search box are how you work through
a large library: *Awaiting* is the to-do list, *Seen* is what you already
decided about.

## Scans, and why the first scan after an upgrade is long

A scan reads every file head-only (a few KB each) and reuses the previous
verdict when the file has not changed. Two things force a fresh diagnosis of
everything:

- the file changed (size or mtime);
- **the server's detectors changed** - a new rule, a fixed reference, a moved
  threshold. Each such change bumps an internal diagnosis version, and the
  next scan re-diagnoses the whole library once rather than show a verdict a
  newer rule would not give. On a NAS this is hours for a large library; the
  scans after that are fast again.

## Backups and rollback

Every rewrite keeps a `.mkvgobak` copy of the original next to the file
(*Keep backups*, on by default). *Roll back* restores one file, *Roll back
all* restores the selected scope. When backups would overflow the volume the
page says so and suggests turning them off - then only the last few are kept
during the run and removed at the end, each repair having been validated
before the swap.

Repairs need the media folders mounted **read-write** in the container (no
`:ro` on the bind mount) - the page tells you which files sit on a read-only
volume.

## Command-line equivalents

The same engine is available from the operator CLI (see `cli.md`):

- `evey admin reindex-no-cue --root <dir> [--apply] [--resync]` - the index
  rebuild, dry-run by default;
- `evey admin validate-streaming --root <dir>` - the read-only streaming
  readiness report.
