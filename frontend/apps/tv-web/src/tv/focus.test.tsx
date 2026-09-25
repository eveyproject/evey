import { afterEach, describe, expect, it, vi } from "vitest";
import { createContext } from "react";
import { cleanup, render } from "@testing-library/react";

// Who asks the spatial library for the focus, and in which order: the library
// itself (geometry, layout) cannot run in jsdom, the arrival rule can.
const focusCalls: string[] = [];
let currentKey = "";
let generated = 0;
let keyMap: Record<string, (number | string)[]> = {};
vi.mock("@noriginmedia/norigin-spatial-navigation", () => ({
  init: () => {},
  setKeyMap: (m: Record<string, (number | string)[]>) => { keyMap = m; },
  setFocus: () => {},
  FocusContext: createContext(""),
  getCurrentFocusKey: () => currentKey,
  useFocusable: ({ focusKey }: { focusKey?: string }) => {
    const key = focusKey ?? `scope-${++generated}`;
    return { ref: { current: null }, focusKey: key, focused: false, focusSelf: () => focusCalls.push(key) };
  },
}));

import { FocusRoot, initTvNavigation, rememberFocusBeforeLeaving } from "./focus";
import { Button } from "../components/ui";

afterEach(() => {
  cleanup();
  focusCalls.length = 0;
  currentKey = "";
  window.location.hash = "";
  vi.useRealTimers();
});

describe("the focus on arriving at a screen", () => {
  // Measured in prod on 2026-09-23: the screen scope focused itself after its
  // children and the library took the child closest to the top-left corner -
  // the title page opened on the audio row instead of Play.
  it("stays on the child that claimed it", () => {
    render(
      <FocusRoot autoFocus>
        <Button focusKey="play" autoFocus onPress={() => {}}>Play</Button>
        <Button focusKey="audio" onPress={() => {}}>Auto</Button>
      </FocusRoot>,
    );
    expect(focusCalls).toEqual(["play"]);
  });

  it("is the scope's own when no child claimed it", () => {
    render(
      <FocusRoot autoFocus focusKey="screen">
        <Button focusKey="a" onPress={() => {}}>A</Button>
      </FocusRoot>,
    );
    expect(focusCalls).toEqual(["screen"]);
  });

  // Measured in prod: going back to the home landed on its header.
  it("comes back to the card that was left, over the screen's default", () => {
    window.location.hash = "#/home";
    currentKey = "home:films:m2";
    rememberFocusBeforeLeaving("#/home");
    render(
      <FocusRoot autoFocus>
        <Button focusKey="home:films:m1" autoFocus onPress={() => {}}>First</Button>
        <Button focusKey="home:films:m2" onPress={() => {}}>Left here</Button>
      </FocusRoot>,
    );
    expect(focusCalls).toEqual(["home:films:m2"]);
  });

  it("settles for the screen when the card that was left is gone", () => {
    vi.useFakeTimers();
    window.location.hash = "#/home";
    currentKey = "home:films:gone";
    rememberFocusBeforeLeaving("#/home");
    render(
      <FocusRoot autoFocus focusKey="screen">
        <Button focusKey="home:films:m1" autoFocus onPress={() => {}}>First</Button>
      </FocusRoot>,
    );
    expect(focusCalls).toEqual([]);
    vi.advanceTimersByTime(500);
    expect(focusCalls).toEqual(["screen"]);
  });

  it("never remembers a key the library generated", () => {
    window.location.hash = "#/media/x";
    currentKey = "sn:focusable-item-12";
    rememberFocusBeforeLeaving("#/media/x");
    render(
      <FocusRoot autoFocus>
        <Button focusKey="play" autoFocus onPress={() => {}}>Play</Button>
      </FocusRoot>,
    );
    expect(focusCalls).toEqual(["play"]);
  });
});

describe("the remote's arrows", () => {
  // The library matches keyCode || code || key: an arrow sent with keyCode 0
  // reaches it as "ArrowUp" and must still move the focus.
  it("are recognised by name as well as by code", () => {
    initTvNavigation();
    expect(keyMap.up).toEqual(expect.arrayContaining([38, "ArrowUp"]));
    expect(keyMap.down).toEqual(expect.arrayContaining([40, "ArrowDown"]));
    expect(keyMap.left).toEqual(expect.arrayContaining([37, "ArrowLeft"]));
    expect(keyMap.right).toEqual(expect.arrayContaining([39, "ArrowRight"]));
    expect(keyMap.enter).toEqual(expect.arrayContaining([13, "Enter"]));
  });
});
