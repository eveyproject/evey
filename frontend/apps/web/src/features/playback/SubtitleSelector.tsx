export type SubtitleStream = {
  index: number;
  trackIndex: number; // 0-based among subtitle streams for ffmpeg -map 0:s:N
  language?: string;
  title?: string;
  isDefault?: boolean;
  isForced?: boolean; // forced track - auto-shown when audio matches its language
  isExternal?: boolean;
  externalPath?: string;
  codec?: string;
  burnIn?: boolean; // true when track requires server-side burn-in (PGS/VobSub)
};
