# Operations harness

The gate builds the image but never runs it. This harness does: it boots the
image the way an operator does and walks the scenarios a real installation
meets, over HTTP only. It is the net under delivery, not under the product -
it does not judge playback quality or load (that is the device campaign and the
loadtest).

## Run it

```sh
make ops              # build the full runtime image as eveyproject/evey:ops-local, then run
make ops OPS_IMG=eveyproject/evey:beta-0.0.1-full   # run against a published image instead
make ops-upgrade      # update from OPS_FROM to OPS_IMG, then roll back (pulls one image, ~2-3 min)
```

Everything runs in a throwaway container on a temp pile; `KEEP=1 make ops`
leaves it up. Logs land in `tests/ops/test-results/` (gitignored):
`run.log`, `server.log`, `container.log`.

## What `make ops` proves (~30 s warm, no GPU)

| Step | What it proves |
| --- | --- |
| `clip` | the image's ffmpeg makes a 30 s, two-audio test file |
| `boot` | empty `/data` to healthy, embedded Postgres + migrations, commit on `/healthz` |
| `setup_scan` | first admin, one library, one scan finds the file |
| `direct_play` | master playlist and first segment serve |
| `parallel8` | eight clients on the master, no 5xx |
| `transcode` | a forced 240p segment comes out of ffmpeg (`X-Evey-Served-By: transcode`) |
| `disk_full` | a 1 MiB cache filled: the server survives (see XFAIL below) |
| `unreachable` | a library root chmod 000 prunes nothing |
| `kill9` | SIGKILL mid-playback, reboot on the same `/data`, login and catalogue intact |

## Marks

- `PASS` / `FAIL` as usual; a `FAIL` fails the run (exit 1).
- `XFAIL` is a scenario pinned to a known defect, referenced inline. It does not
  fail the run. `XPASS` does - the defect is gone and the mark must be removed.

Known defects currently marked:
- `disk_full` - a full transcode cache restarts ffmpeg on every segment request
  (503 forever) instead of refusing once.
- `rollback_restore` (ops-upgrade) - `evey admin restore-backup` replays a plain
  dump over an existing schema, aborts on the first `CREATE TABLE`, then hangs.
