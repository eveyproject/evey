export const canPlayHevc = (() => {
  const v = document.createElement("video");
  // "maybe" is unreliable (Firefox returns it without actual HEVC support)
  return v.canPlayType('video/mp4; codecs="hvc1"') === "probably"
    || v.canPlayType('video/mp4; codecs="hev1"') === "probably";
})();
