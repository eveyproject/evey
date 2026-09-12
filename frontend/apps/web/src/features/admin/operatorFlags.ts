// The operator flags that live in server_config as free-text rows.
//
// These are read at boot by cmd/evey/wire.go through resolveFlag(dbKey, envVar),
// which consults server_config first and the environment second. They are
// settable from the admin panel already - the problem this catalogue solves is
// that you had to KNOW the exact key to type it into the add-entry row, and
// nothing listed them. A key nobody can spell is not configurable.
//
// The server remains the source of truth: cmd/evey/operator_flags_guard_test.go
// extracts every resolveFlag key from wire.go and fails if one is missing here,
// so a new flag cannot ship undiscoverable.
//
// All of them are consumed at boot, hence "applied on restart" throughout.

export interface OperatorFlag {
  key: string;
  envVar: string;
  /** What the value looks like. Free text, shown next to the key. */
  valueHint: string;
}

export const OPERATOR_FLAGS: OperatorFlag[] = [
  {
    key: "downloads.quality_choice",
    envVar: "EVEY_DOWNLOAD_QUALITY_CHOICE",
    valueHint: "on (default: off)",
  },
  {
    key: "downloads.reduce_above_bytes",
    envVar: "EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES",
    valueHint: "bytes, > 0 (default: 4294967296 = 4 GiB)",
  },
  {
    key: "transcoding.compat_download",
    envVar: "EVEY_COMPAT_DOWNLOAD",
    valueHint: "off (default: on)",
  },
  {
    key: "transcoding.keyframe_backfill",
    envVar: "EVEY_KEYFRAME_BACKFILL",
    valueHint: "off (default: on)",
  },
  {
    key: "transcoding.title_probe",
    envVar: "EVEY_TITLE_PROBE",
    valueHint: "off (default: on)",
  },
  {
    key: "transcoding.keyframe_backfill_batch",
    envVar: "EVEY_KEYFRAME_BACKFILL_BATCH",
    valueHint: "items per pass, > 0",
  },
  {
    key: "transcoding.keyframe_backfill_pause_ms",
    envVar: "EVEY_KEYFRAME_BACKFILL_PAUSE_MS",
    valueHint: "milliseconds between items, >= 0",
  },
  {
    key: "transcoding.keyframe_backfill_idle_sec",
    envVar: "EVEY_KEYFRAME_BACKFILL_IDLE_SEC",
    valueHint: "seconds between idle passes, > 0",
  },
  {
    key: "transcoding.level_backfill",
    envVar: "EVEY_LEVEL_BACKFILL",
    valueHint: "off (default: on)",
  },
  {
    key: "transcoding.level_backfill_batch",
    envVar: "EVEY_LEVEL_BACKFILL_BATCH",
    valueHint: "items per pass, > 0",
  },
  {
    key: "transcoding.level_backfill_pause_ms",
    envVar: "EVEY_LEVEL_BACKFILL_PAUSE_MS",
    valueHint: "milliseconds between items, >= 0",
  },
  {
    key: "transcoding.level_backfill_idle_sec",
    envVar: "EVEY_LEVEL_BACKFILL_IDLE_SEC",
    valueHint: "seconds between idle passes, > 0",
  },
  {
    key: "transcoding.pause_bg_on_playback",
    envVar: "EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK",
    valueHint: "off (default: on)",
  },
  {
    key: "transcoding.snapshot_fastpath",
    envVar: "EVEY_SNAPSHOT_FASTPATH",
    valueHint: "off (default: on)",
  },
];

// findOperatorFlag matches a key the operator has typed, trimmed. Returns
// undefined for a key outside the catalogue - free-text rows stay allowed,
// the catalogue only helps with the ones we know about.
export function findOperatorFlag(key: string): OperatorFlag | undefined {
  const k = key.trim();
  return OPERATOR_FLAGS.find((f) => f.key === k);
}
