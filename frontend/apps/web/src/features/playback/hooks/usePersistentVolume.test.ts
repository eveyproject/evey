import { describe, it, expect, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { usePersistentVolume } from "./usePersistentVolume";
import { getPlayerVolume } from "@/hooks/usePlaybackPrefs";

function mountOn(video: HTMLVideoElement) {
  return renderHook(() => usePersistentVolume({ current: video }));
}

function setVolume(video: HTMLVideoElement, v: number) {
  video.volume = v;
  video.dispatchEvent(new Event("volumechange"));
}

describe("usePersistentVolume", () => {
  beforeEach(() => localStorage.clear());

  it("carries the level chosen on one title to the next title's fresh element", () => {
    const first = document.createElement("video");
    const a = mountOn(first);
    expect(first.volume).toBe(1);
    setVolume(first, 0.3);
    a.unmount();

    const second = document.createElement("video");
    expect(second.volume).toBe(1);
    mountOn(second);
    expect(second.volume).toBe(0.3);
  });

  it("goes back to full volume once the user raises it again", () => {
    const first = document.createElement("video");
    const a = mountOn(first);
    setVolume(first, 0.3);
    setVolume(first, 1);
    a.unmount();
    expect(localStorage.getItem("evey:volume")).toBeNull();

    const second = document.createElement("video");
    mountOn(second);
    expect(second.volume).toBe(1);
  });

  it("does not store a mute: a muted element keeps its level", () => {
    const video = document.createElement("video");
    mountOn(video);
    setVolume(video, 0.6);
    video.muted = true;
    video.dispatchEvent(new Event("volumechange"));
    expect(getPlayerVolume()).toBe(0.6);
  });

  it("ignores a corrupted stored value", () => {
    localStorage.setItem("evey:volume", "loud");
    const video = document.createElement("video");
    mountOn(video);
    expect(video.volume).toBe(1);
    localStorage.setItem("evey:volume", "7");
    expect(getPlayerVolume()).toBe(1);
  });

  it("stops listening to an element it left", () => {
    const video = document.createElement("video");
    const a = mountOn(video);
    a.unmount();
    setVolume(video, 0.2);
    expect(getPlayerVolume()).toBe(1);
  });
});
