export type AudioStream = {
  index: number;      // absolute stream index from proto
  trackIndex: number; // 0-based among audio streams (for ffmpeg -map 0:a:N)
  language?: string;
  title?: string;
  isDefault?: boolean;
};
