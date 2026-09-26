// The track shapes every player lists: one definition in @evey/api.
export type { PlayerAudioStream as AudioStream } from "@evey/api";

// Both formatters now live in @evey/api: they existed in six copies across the
// three apps and had drifted even inside this one (this file said "2.0" where
// StreamInfoPanel said "Stereo", same track, same screen). Re-exported here so
// this module's existing importers keep their entry point.
export { formatAudioTrackLabel, formatChannelLayout } from "@evey/api";
