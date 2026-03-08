export type SubtitleStream = {
  index: number;
  trackIndex: number; // 0-based among subtitle streams for ffmpeg -map 0:s:N
  language?: string;
  title?: string;
  isDefault?: boolean;
  isExternal?: boolean;
  externalPath?: string;
};
