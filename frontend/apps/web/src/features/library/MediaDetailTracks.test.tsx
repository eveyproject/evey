import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { MediaDetailTracks, subtitleTrackLabel } from "./MediaDetailTracks";

const streams = [
  { index: 0, type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264" },
  { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "ac3", language: "fre", title: "VFF", channels: 6 },
  { index: 2, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", language: "eng", channels: 2 },
  { index: 3, type: "MEDIA_STREAM_TYPE_SUBTITLE", codec: "subrip", language: "fre", isForced: true },
  { index: 4, type: "MEDIA_STREAM_TYPE_SUBTITLE", codec: "hdmv_pgs_subtitle", language: "eng", title: "Full" },
] as const;

describe("MediaDetailTracks", () => {
  it("lists every audio and subtitle track behind an Auto entry, subtitles with an Off entry too", () => {
    render(<MediaDetailTracks streams={[...streams]} pick={{}} onChange={vi.fn()} />);
    const audio = screen.getByTestId("media-tracks-audio") as HTMLSelectElement;
    const subtitle = screen.getByTestId("media-tracks-subtitle") as HTMLSelectElement;
    expect(Array.from(audio.options).map((o) => o.value)).toEqual(["auto", "1", "2"]);
    expect(Array.from(subtitle.options).map((o) => o.value)).toEqual(["auto", "off", "3", "4"]);
    expect(audio.value).toBe("auto");
    expect(audio.options[1].textContent).toBe("VFF - 5.1");
  });

  it("reports absolute stream indices, off, and back to auto", () => {
    const onChange = vi.fn();
    render(<MediaDetailTracks streams={[...streams]} pick={{ audio: 1 }} onChange={onChange} />);
    fireEvent.change(screen.getByTestId("media-tracks-subtitle"), { target: { value: "off" } });
    expect(onChange).toHaveBeenLastCalledWith({ audio: 1, subtitle: "off" });
    fireEvent.change(screen.getByTestId("media-tracks-subtitle"), { target: { value: "4" } });
    expect(onChange).toHaveBeenLastCalledWith({ audio: 1, subtitle: 4 });
    fireEvent.change(screen.getByTestId("media-tracks-audio"), { target: { value: "auto" } });
    expect(onChange).toHaveBeenLastCalledWith({ audio: undefined });
  });

  it("stays out of the way when there is nothing to choose", () => {
    const { container } = render(
      <MediaDetailTracks streams={[streams[0], streams[1]]} pick={{}} onChange={vi.fn()} />,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("names a subtitle by title, then language, and flags a forced track", () => {
    expect(subtitleTrackLabel({ index: 4, title: "Full", language: "eng" }, "Track 4", "Forced")).toBe("Full");
    expect(subtitleTrackLabel({ index: 3, language: "fre", isForced: true }, "Track 3", "Forced")).toBe("fre - Forced");
    expect(subtitleTrackLabel({ index: 5 }, "Track 5", "Forced")).toBe("Track 5");
  });
});
