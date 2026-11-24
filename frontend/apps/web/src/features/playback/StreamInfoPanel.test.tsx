import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { StreamInfoPanel } from "./StreamInfoPanel";

// The reported bug: an E-AC-3 5.1 track inside Matroska, on a browser that
// decodes AC-3 in WebAssembly. The server never sees the wasm capability (a
// browser advertising eac3 would get the audio COPIED into the muxed stream,
// which MSE rejects), so it flags audio_codec_unsupported and answers
// play_method=transcode - while the session copies the video, copies the audio
// into an out-of-band rendition, and only swaps the container. The panel then
// announced a transcode over a video badged "copié" and an audio badged
// "décodé client", listing a remux as its only reason.
const WASM_SESSION = {
  playMethod: "PLAY_METHOD_TRANSCODE",
  container: "matroska",
  bitrate: "6500000",
  videoDecision: "copy",
  audioDecision: "transcode",
  transcodeReasons: ["audio_codec_unsupported", "container_incompatible"],
  mediaStreams: [
    { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264", width: 1920, height: 1080 },
    { type: "MEDIA_STREAM_TYPE_AUDIO", codec: "eac3", channels: 6 },
  ],
};

// The session's two witnesses for each case: what the server sent, what the
// browser decodes. The verdict above stays the same in all of them.
const COPIED_VIDEO = { engine: "mkvgo", copy: true, codec: "h264", height: 1080 };
const WASM_DECODE = {
  servedVideo: COPIED_VIDEO,
  servedAudio: { engine: "ffmpeg", copy: true, track: 0, codec: "eac3", channels: 6 },
  played: { source: "hls", audioTrack: 0, audioCodec: "ec-3", audioDecoder: "wasm" },
};
const AAC_ENCODE = {
  servedVideo: COPIED_VIDEO,
  servedAudio: { engine: "ffmpeg", copy: false, track: 0, codec: "aac", sourceCodec: "eac3", channels: 6 },
  played: { source: "hls", audioTrack: 0, audioCodec: "mp4a.40.2", audioDecoder: "native" },
};

function renderPanel(streamInfo: object, witness?: object) {
  return render(
    <StreamInfoPanel
      streamInfo={streamInfo as never}
      open
      onToggle={vi.fn()}
      witness={witness as never}
    />,
  );
}

describe("StreamInfoPanel - play method honesty", () => {
  it("names a copy+wasm session a remux, never a transcode", () => {
    renderPanel(WASM_SESSION, WASM_DECODE);

    expect(screen.getByText("Remux")).toBeInTheDocument();
    expect(screen.queryByText("Transcode")).toBeNull();
  });

  it("heads the reason list with 'why not direct play' when nothing is re-encoded", () => {
    renderPanel(WASM_SESSION, WASM_DECODE);

    expect(screen.getByText("Why not direct play")).toBeInTheDocument();
    expect(screen.queryByText("Transcode reasons")).toBeNull();
  });

  it("keeps only the reasons that survive the wasm path", () => {
    renderPanel(WASM_SESSION, WASM_DECODE);

    expect(screen.getByText("Container not playable as-is (repackaged to HLS)")).toBeInTheDocument();
    expect(screen.queryByText("Audio codec this player cannot decode")).toBeNull();
  });

  // The server encodes the audio to AAC (no wasm decoder, or the relay
  // latched it - in prod, minutes into a film): the transcode is real
  // and is named, whatever the negotiation said.
  it("says transcode once the audio is really re-encoded", () => {
    renderPanel(WASM_SESSION, AAC_ENCODE);

    expect(screen.getByText("Transcode")).toBeInTheDocument();
    expect(screen.getByText("Transcode reasons")).toBeInTheDocument();
    expect(screen.getByText("Audio codec this player cannot decode")).toBeInTheDocument();
    expect(screen.getByText("EAC3→AAC")).toBeInTheDocument();
  });

  it("labels the container row as the SOURCE container, which the remux replaces", () => {
    renderPanel(WASM_SESSION, WASM_DECODE);

    expect(screen.getByText("Source container")).toBeInTheDocument();
    expect(screen.getByText("MATROSKA")).toBeInTheDocument();
  });

  // The verdict is not evidence: before the first bytes the panel says it
  // waits, and claims neither a method nor a reason.
  it("names no method before any byte was served", () => {
    renderPanel(WASM_SESSION);

    expect(screen.queryByText("Transcode")).toBeNull();
    expect(screen.queryByText("Remux")).toBeNull();
    expect(screen.queryByText("Transcode reasons")).toBeNull();
  });

  it("names a direct play by the file the browser reads", () => {
    renderPanel(
      {
        playMethod: "PLAY_METHOD_DIRECT_PLAY",
        mediaStreams: [{ type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264" }],
      },
      { servedVideo: { engine: "file", copy: true, track: -1, codec: "h264" }, servedAudio: { engine: "file", copy: true, track: -1 } },
    );

    expect(screen.getByText("Direct Play")).toBeInTheDocument();
  });
});
