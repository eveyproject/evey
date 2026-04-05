import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...args: unknown[]) => eveyFetchMock(...args) }));
vi.mock("react-native-google-cast", () => { throw new Error("not installed"); });

import { useCast, type CastDevice } from "./useCast";

const dlnaDevice: CastDevice = { id: "dlna:r1", name: "Living Room TV", type: "dlna" };

beforeEach(() => { eveyFetchMock.mockReset(); });

describe("useCast", () => {
  describe("discover", () => {
    it("returns DLNA renderers from API", async () => {
      eveyFetchMock.mockResolvedValue([{ id: "r1", name: "Living Room TV", location: "http://192.168.1.10", controlUrl: "http://192.168.1.10/ctrl" }]);

      const { result } = renderHook(() => useCast());
      let discovered: CastDevice[] = [];
      await act(async () => { discovered = await result.current.discover(); });

      expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/dlna/renderers", method: "GET" });
      expect(discovered).toEqual([{ id: "dlna:r1", name: "Living Room TV", type: "dlna" }]);
    });

    it("returns empty array on error", async () => {
      eveyFetchMock.mockRejectedValue(new Error("network"));

      const { result } = renderHook(() => useCast());
      let discovered: CastDevice[] = [];
      await act(async () => { discovered = await result.current.discover(); });

      expect(discovered).toEqual([]);
    });
  });

  describe("castTo", () => {
    it("calls DLNA play endpoint and sets casting state", async () => {
      eveyFetchMock.mockResolvedValue(undefined);

      const { result } = renderHook(() => useCast());
      await act(async () => {
        await result.current.castTo(dlnaDevice, "http://stream/video.mp4", "My Movie");
      });

      expect(eveyFetchMock).toHaveBeenCalledWith({
        url: "/api/v1/dlna/play",
        method: "POST",
        data: { rendererId: "r1", mediaUrl: "http://stream/video.mp4", title: "My Movie" },
      });
      expect(result.current.casting).toBe(true);
      expect(result.current.deviceName).toBe("Living Room TV");
    });

    it("sets casting to false on error", async () => {
      eveyFetchMock.mockRejectedValue(new Error("fail"));

      const { result } = renderHook(() => useCast());
      await act(async () => {
        await result.current.castTo(dlnaDevice, "http://stream/video.mp4", "My Movie");
      });

      expect(result.current.casting).toBe(false);
    });
  });

  describe("stopCast", () => {
    it("calls DLNA stop endpoint and resets state", async () => {
      eveyFetchMock.mockResolvedValue(undefined);

      const { result } = renderHook(() => useCast());
      await act(async () => {
        await result.current.castTo(dlnaDevice, "http://stream/video.mp4", "Movie");
      });
      eveyFetchMock.mockClear();

      await act(async () => { await result.current.stopCast(); });

      expect(eveyFetchMock).toHaveBeenCalledWith({
        url: "/api/v1/dlna/stop",
        method: "POST",
        data: { rendererId: "r1" },
      });
      expect(result.current.casting).toBe(false);
      expect(result.current.deviceName).toBeNull();
    });

    it("does nothing if no active renderer", async () => {
      const { result } = renderHook(() => useCast());
      await act(async () => { await result.current.stopCast(); });

      expect(eveyFetchMock).not.toHaveBeenCalled();
    });
  });
});
