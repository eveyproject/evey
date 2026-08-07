import { describe, it, expect } from "vitest";
import {
  buildCastNextUrl,
  buildNextMasterUrl,
  parseCastNextResponse,
  shouldFetchNext,
  upNextLabel,
  FETCH_NEXT_REMAINING_SEC,
} from "./castNext";

const MASTER = "https://evey.example/api/v1/media/mi-1/hls/master.m3u8?token=tok-1&wasmAudio=1&maxKbps=8000&abr=floor";

describe("buildCastNextUrl", () => {
  it("targets cast-next on the same media, keeping the full query", () => {
    expect(buildCastNextUrl(MASTER)).toBe(
      "https://evey.example/api/v1/media/mi-1/cast-next?token=tok-1&wasmAudio=1&maxKbps=8000&abr=floor",
    );
  });

  it("returns null without a token (nothing to authenticate with)", () => {
    expect(buildCastNextUrl("https://evey.example/api/v1/media/mi-1/hls/master.m3u8")).toBeNull();
  });

  it("returns null on a foreign URL", () => {
    expect(buildCastNextUrl("https://cdn.example/video.mp4?token=t")).toBeNull();
    expect(buildCastNextUrl("not a url")).toBeNull();
  });
});

describe("parseCastNextResponse", () => {
  it("accepts a full payload", () => {
    expect(
      parseCastNextResponse({
        next_media_item_id: "mi-2",
        token: "tok-2",
        title: "Some Episode",
        season_number: 2,
        episode_number: 5,
        series_title: "Some Series",
      }),
    ).toEqual({
      nextMediaItemId: "mi-2",
      token: "tok-2",
      title: "Some Episode",
      seasonNumber: 2,
      episodeNumber: 5,
      seriesTitle: "Some Series",
    });
  });

  it("degrades missing metadata to zero values but keeps the switch", () => {
    const info = parseCastNextResponse({ next_media_item_id: "mi-2", token: "tok-2" });
    expect(info).not.toBeNull();
    expect(info?.title).toBe("");
    expect(info?.seasonNumber).toBe(0);
  });

  it("refuses payloads without id or token", () => {
    expect(parseCastNextResponse({ token: "tok-2" })).toBeNull();
    expect(parseCastNextResponse({ next_media_item_id: "mi-2" })).toBeNull();
    expect(parseCastNextResponse(null)).toBeNull();
    expect(parseCastNextResponse("nope")).toBeNull();
  });
});

describe("buildNextMasterUrl", () => {
  it("swaps id and token, normalizes onto the master path, drops rescue params", () => {
    const url = buildNextMasterUrl(MASTER, { nextMediaItemId: "mi-2", token: "tok-2" });
    expect(url).toBe("https://evey.example/api/v1/media/mi-2/hls/master.m3u8?token=tok-2&wasmAudio=1");
  });

  it("normalizes a direct-play current URL onto the master path", () => {
    const url = buildNextMasterUrl("https://evey.example/api/v1/media/mi-1/stream?token=tok-1", {
      nextMediaItemId: "mi-2",
      token: "tok-2",
    });
    expect(url).toBe("https://evey.example/api/v1/media/mi-2/hls/master.m3u8?token=tok-2");
  });

  it("returns null on a foreign URL", () => {
    expect(buildNextMasterUrl("https://cdn.example/video.mp4", { nextMediaItemId: "x", token: "t" })).toBeNull();
  });
});

describe("shouldFetchNext", () => {
  it("fires once inside the remaining-time window", () => {
    expect(shouldFetchNext(3600 - FETCH_NEXT_REMAINING_SEC, 3600, false)).toBe(true);
    expect(shouldFetchNext(3600 - FETCH_NEXT_REMAINING_SEC - 1, 3600, false)).toBe(false);
  });

  it("never refires once fetched", () => {
    expect(shouldFetchNext(3590, 3600, true)).toBe(false);
  });

  it("ignores short items and degenerate positions", () => {
    expect(shouldFetchNext(100, 120, false)).toBe(false);
    expect(shouldFetchNext(0, 3600, false)).toBe(false);
    expect(shouldFetchNext(NaN, 3600, false)).toBe(false);
    expect(shouldFetchNext(3590, Infinity, false)).toBe(false);
  });
});

describe("upNextLabel", () => {
  const info = {
    nextMediaItemId: "mi-2",
    token: "t",
    title: "Some Episode",
    seasonNumber: 2,
    episodeNumber: 5,
    seriesTitle: "Some Series",
  };

  it("renders the French heading on a fr device", () => {
    expect(upNextLabel(info, "fr-FR")).toEqual({ heading: "À suivre", title: "Some Series · S02E05 · Some Episode" });
  });

  it("renders the English heading elsewhere", () => {
    expect(upNextLabel(info, "en-US").heading).toBe("Up next");
  });

  it("drops the missing parts of the label", () => {
    expect(upNextLabel({ ...info, seasonNumber: 0, episodeNumber: 0, seriesTitle: "" }, "en").title).toBe("Some Episode");
    expect(upNextLabel({ ...info, seasonNumber: 0 }, "en").title).toBe("Some Series · E05 · Some Episode");
  });
});
