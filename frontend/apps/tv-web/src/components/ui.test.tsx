import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/react";

// The spatial library is not under test: a field is "focused" as far as it
// is concerned, and Enter/Back are what the browser delivers to the input.
vi.mock("../tv/focus", () => ({
  useFocusable: () => ({ ref: { current: null }, focused: true, focusSelf: () => {} }),
}));

import { TextField } from "./ui";

function renderField(onSubmit = vi.fn()) {
  const windowKeys: string[] = [];
  const listener = (e: KeyboardEvent) => windowKeys.push(e.key);
  window.addEventListener("keydown", listener);
  const utils = render(<TextField value="abc" onChange={() => {}} placeholder="Name" onSubmit={onSubmit} />);
  const input = utils.getByPlaceholderText("Name") as HTMLInputElement;
  input.focus();
  return { input, onSubmit, windowKeys, cleanup: () => window.removeEventListener("keydown", listener) };
}

// Enter from the on-screen keyboard used to bubble to the window, where the
// spatial library saw it on the field it still marked focused and handed the
// keys straight back to it: the keyboard reopened on every Done (LG
// simulator, 2026-09-22). The field now keeps Enter and Back to itself.
describe("TextField keys", () => {
  it("submits and leaves the field on Enter without letting the window see it", () => {
    const { input, onSubmit, windowKeys, cleanup } = renderField();
    expect(document.activeElement).toBe(input);
    fireEvent.keyDown(input, { key: "Enter", keyCode: 13 });
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(document.activeElement).not.toBe(input);
    expect(windowKeys).toEqual([]);
    cleanup();
  });

  it("leaves the field on the TV back key without submitting or reaching the screen", () => {
    const { input, onSubmit, windowKeys, cleanup } = renderField();
    fireEvent.keyDown(input, { key: "GoBack", keyCode: 461 });
    expect(onSubmit).not.toHaveBeenCalled();
    expect(document.activeElement).not.toBe(input);
    expect(windowKeys).toEqual([]);
    cleanup();
  });

  it("keeps Backspace as a delete inside the field", () => {
    const { input, windowKeys, cleanup } = renderField();
    fireEvent.keyDown(input, { key: "Backspace", keyCode: 8 });
    expect(document.activeElement).toBe(input);
    expect(windowKeys).toEqual(["Backspace"]);
    cleanup();
  });
});
