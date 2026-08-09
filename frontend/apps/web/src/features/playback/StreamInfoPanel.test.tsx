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

function renderPanel(streamInfo: object, wasmAudioActive: boolean) {
  return render(
    <StreamInfoPanel
      streamInfo={streamInfo as never}
      open
      onToggle={vi.fn()}
      wasmAudioActive={wasmAudioActive}
    />,
  );
}

describe("StreamInfoPanel - play method honesty", () => {
  it("names a copy+wasm session a remux, never a transcode", () => {
    renderPanel(WASM_SESSION, true);

    expect(screen.getByText("Remux")).toBeInTheDocument();
    expect(screen.queryByText("Transcode")).toBeNull();
  });

  it("heads the reason list with 'why not direct play' when nothing is re-encoded", () => {
    renderPanel(WASM_SESSION, true);

    expect(screen.getByText("Why not direct play")).toBeInTheDocument();
    expect(screen.queryByText("Transcode reasons")).toBeNull();
  });

  it("keeps only the reasons that survive the wasm path", () => {
    renderPanel(WASM_SESSION, true);

    expect(screen.getByText("Container not playable as-is (repackaged to HLS)")).toBeInTheDocument();
    expect(screen.queryByText("Audio codec this player cannot decode")).toBeNull();
  });

  // Same session, wasm decoder gone (unsupported browser, or a latched
  // fallback): the server's AAC transcode is real and must be named.
  it("says transcode once the audio is really re-encoded", () => {
    renderPanel(WASM_SESSION, false);

    expect(screen.getByText("Transcode")).toBeInTheDocument();
    expect(screen.getByText("Transcode reasons")).toBeInTheDocument();
    expect(screen.getByText("Audio codec this player cannot decode")).toBeInTheDocument();
  });

  it("labels the container row as the SOURCE container, which the remux replaces", () => {
    renderPanel(WASM_SESSION, true);

    expect(screen.getByText("Source container")).toBeInTheDocument();
    expect(screen.getByText("MATROSKA")).toBeInTheDocument();
  });

  it("leaves direct play alone", () => {
    renderPanel(
      {
        playMethod: "PLAY_METHOD_DIRECT_PLAY",
        mediaStreams: [{ type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264" }],
      },
      false,
    );

    expect(screen.getByText("Direct Play")).toBeInTheDocument();
  });
});
