export const canPlayHevc = (() => {
  const v = document.createElement("video");
  // "maybe" is unreliable (Firefox returns it without actual HEVC support)
  return v.canPlayType('video/mp4; codecs="hvc1"') === "probably"
    || v.canPlayType('video/mp4; codecs="hev1"') === "probably";
})();

export interface BrowserCapabilities {
  videoCodecs: string[];
  audioCodecs: string[];
  containers: string[];
}

export function getBrowserCapabilities(): BrowserCapabilities {
  const v = document.createElement("video");
  const canPlay = (type: string) => v.canPlayType(type) !== "";

  const videoCodecs: string[] = [];
  if (canPlay('video/mp4; codecs="avc1.42E01E"')) videoCodecs.push("h264");
  if (canPlayHevc) videoCodecs.push("hevc");
  if (canPlay('video/webm; codecs="vp9"')) videoCodecs.push("vp9");
  if (canPlay('video/mp4; codecs="av01.0.00M.08"')) videoCodecs.push("av1");

  const audioCodecs: string[] = ["aac", "mp3", "opus"];
  if (canPlay('audio/mp4; codecs="ac-3"') || canPlay('video/mp4; codecs="ac-3"')) audioCodecs.push("ac3");
  if (canPlay('audio/mp4; codecs="ec-3"') || canPlay('video/mp4; codecs="ec-3"')) audioCodecs.push("eac3");
  if (canPlay("audio/flac")) audioCodecs.push("flac");

  const containers: string[] = [];
  if (canPlay("video/mp4")) containers.push("mp4");
  if (canPlay("video/webm")) containers.push("webm");
  if (canPlay("video/ogg")) containers.push("ogg");

  return { videoCodecs, audioCodecs, containers };
}
