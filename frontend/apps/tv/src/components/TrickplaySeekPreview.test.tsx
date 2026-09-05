import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  StyleSheet: { create: (s: any) => s },
}));

// expo-image's Image reports its natural size via onLoad asynchronously (the
// native decode happens off the render pass) - this mock fires it from an
// effect for the same reason, with a fixed sheet size, so the component's
// second render (once sheetSizeCache is populated) is the one assertions run
// against.
vi.mock("expo-image", () => ({
  Image: (props: any) => {
    React.useEffect(() => {
      props.onLoad?.({ source: { url: props.source?.uri, width: 480, height: 90 } });
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    return React.createElement("img", { alt: "", src: props.source?.uri, style: props.style });
  },
}));

const eveyFetchRawMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetchRaw: (...args: unknown[]) => eveyFetchRawMock(...args),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { border: "#333", overlay: "rgba(0,0,0,0.5)" },
    radii: { sm: 4 },
  }),
}));

const useServerUrlMock = vi.fn(() => "http://srv.test");
vi.mock("../hooks/useServerUrl", () => ({
  useServerUrl: () => useServerUrlMock(),
}));

vi.mock("../hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => ({ Authorization: "Bearer TOKEN" }),
}));

import { TrickplaySeekPreview } from "./TrickplaySeekPreview";

const REALISTIC_VTT = `WEBVTT

00:00:00.000 --> 00:00:10.000
/api/v1/media/{id}/trickplay/trickplay_001.webp?token=abc#xywh=0,0,160,90

00:00:10.000 --> 00:00:20.000
/api/v1/media/{id}/trickplay/trickplay_001.webp?token=abc#xywh=160,0,160,90

00:00:20.000 --> 00:00:30.000
/api/v1/media/{id}/trickplay/trickplay_001.webp?token=abc#xywh=320,0,160,90
`;

function mockManifestOk(text: string) {
  eveyFetchRawMock.mockResolvedValue({ text: async () => text } as Response);
}

function mockManifestError() {
  eveyFetchRawMock.mockRejectedValue(new Error("network error"));
}

beforeEach(() => {
  eveyFetchRawMock.mockReset();
  useServerUrlMock.mockReturnValue("http://srv.test");
});

describe("TrickplaySeekPreview", () => {
  it("renders nothing and fetches nothing while not seeking (previewTimeSec null)", () => {
    mockManifestOk(REALISTIC_VTT);
    const { container } = render(<TrickplaySeekPreview mediaId="mi-rest" previewTimeSec={null} />);
    expect(container.firstChild).toBeNull();
    expect(eveyFetchRawMock).not.toHaveBeenCalled();
  });

  it("fetches the manifest lazily once a seek starts, and renders the matching tile", async () => {
    mockManifestOk(REALISTIC_VTT);
    const { container } = render(<TrickplaySeekPreview mediaId="mi-seek" previewTimeSec={5} />);

    await waitFor(() => {
      expect(eveyFetchRawMock).toHaveBeenCalledWith(
        expect.objectContaining({ url: "/api/v1/media/mi-seek/trickplay", method: "GET" }),
      );
    });

    await waitFor(() => {
      const img = container.querySelector("img");
      expect(img).not.toBeNull();
    });
  });

  it("crops the tile with a negative offset sized to the sheet's native pixels", async () => {
    mockManifestOk(REALISTIC_VTT);
    const { container } = render(<TrickplaySeekPreview mediaId="mi-crop" previewTimeSec={15} />);

    await waitFor(() => {
      const img = container.querySelector("img") as HTMLImageElement | null;
      expect(img).not.toBeNull();
    });

    const img = container.querySelector("img") as HTMLImageElement;
    // The 15s mark falls in the second cue (x=160,y=0,w=160,h=90); the probe
    // mock always reports an 480x90 sheet.
    expect(img.style.left).toBe("-160px");
    expect(img.style.top).toBe("0px");
    expect(img.style.width).toBe("480px");
    expect(img.style.height).toBe("90px");
  });

  it("renders nothing when the manifest fetch fails - the seek continues without a preview", async () => {
    mockManifestError();
    const { container } = render(<TrickplaySeekPreview mediaId="mi-fail" previewTimeSec={5} />);

    await waitFor(() => {
      expect(eveyFetchRawMock).toHaveBeenCalled();
    });

    expect(container.firstChild).toBeNull();
  });

  it("renders nothing for an empty manifest (no trickplay generated for this media)", async () => {
    mockManifestOk("WEBVTT\n");
    const { container } = render(<TrickplaySeekPreview mediaId="mi-empty" previewTimeSec={5} />);

    await waitFor(() => {
      expect(eveyFetchRawMock).toHaveBeenCalled();
    });

    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when the preview time falls outside every cue", async () => {
    mockManifestOk(REALISTIC_VTT);
    const { container } = render(<TrickplaySeekPreview mediaId="mi-nomatch" previewTimeSec={999} />);

    await waitFor(() => {
      expect(eveyFetchRawMock).toHaveBeenCalled();
    });

    expect(container.firstChild).toBeNull();
  });
});

// A seek that is abandoned before its manifest lands - the viewer stops
// seeking, leaves the player, or the screen remounts - used to poison that
// title for the rest of the app session.
//
// The guard set is added to BEFORE the fetch and only cleaned on failure or on
// LRU eviction from the cue cache. A cancelled fetch reached neither: it
// returned early, so the id stayed in the set with nothing stored behind it.
// Two consequences, one for the viewer and one for the box: seek previews were
// silently dead for that title until the app restarted, and the set grew
// without bound on a device that stays up for weeks with the least memory of
// the three.
describe("TrickplaySeekPreview, an abandoned fetch", () => {
  it("lets the title fetch again after a seek is abandoned mid-flight", async () => {
    let release!: (v: { text: () => Promise<string> }) => void;
    eveyFetchRawMock.mockReturnValueOnce(
      new Promise((res) => {
        release = res as (v: { text: () => Promise<string> }) => void;
      }),
    );

    const { unmount } = render(<TrickplaySeekPreview mediaId="mi-abandoned" previewTimeSec={5} />);
    await waitFor(() => expect(eveyFetchRawMock).toHaveBeenCalledTimes(1));

    // The viewer leaves while the manifest is still in flight, and it lands
    // after the component is gone.
    unmount();
    release({ text: async () => REALISTIC_VTT });
    // Two awaits deep (the response, then its text) before the component's
    // cancelled branch runs - a microtask turn is not enough.
    await new Promise((r) => setTimeout(r, 0));

    // Coming back to the same title must fetch again - the first attempt
    // stored nothing.
    mockManifestOk(REALISTIC_VTT);
    const { container } = render(<TrickplaySeekPreview mediaId="mi-abandoned" previewTimeSec={5} />);
    await waitFor(() => expect(eveyFetchRawMock).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(container.querySelector("img")).not.toBeNull());
  });
});
