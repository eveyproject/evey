export type AudioStream = {
  index: number;      // absolute stream index from proto
  trackIndex: number; // 0-based among audio streams (for ffmpeg -map 0:a:N)
  language?: string;
  title?: string;
  isDefault?: boolean;
  codec?: string;     // source codec (ac3/eac3/aac/...) - drives the wasm-audio path
  channels?: number;  // channel count, rendered as a 5.1/7.1 style layout
};

// Both formatters now live in @evey/api: they existed in six copies across the
// three apps and had drifted even inside this one (this file said "2.0" where
// StreamInfoPanel said "Stereo", same track, same screen). Re-exported here so
// this module's existing importers keep their entry point.
export { formatAudioTrackLabel, formatChannelLayout } from "@evey/api";
