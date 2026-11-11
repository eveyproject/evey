// The track selection of a stream-info request. The older fields cannot name
// stream 0 (the server reads their 0 as "none"), and a file that stores its
// audio before the picture has its first audio track there: the selected_*
// fields carry the index with its presence. Both are sent, so an older
// server still reads the selection it understands.
export function streamSelectionParams(audioStreamIndex: number | undefined, subtitleStreamIndex: number | undefined): {
  audioStreamIndex?: number;
  subtitleStreamIndex?: number;
  selectedAudioStreamIndex?: number;
  selectedSubtitleStreamIndex?: number;
} {
  const out: ReturnType<typeof streamSelectionParams> = {};
  if (audioStreamIndex != null && audioStreamIndex >= 0) {
    out.audioStreamIndex = audioStreamIndex;
    out.selectedAudioStreamIndex = audioStreamIndex;
  }
  if (subtitleStreamIndex != null && subtitleStreamIndex >= 0) {
    out.subtitleStreamIndex = subtitleStreamIndex;
    out.selectedSubtitleStreamIndex = subtitleStreamIndex;
  }
  return out;
}
