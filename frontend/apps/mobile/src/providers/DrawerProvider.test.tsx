import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

// Springs are recorded rather than run: the drawer's contract is about WHEN
// isOpen flips relative to the animation, so the test drives the completion
// callback by hand.
const anim = vi.hoisted(() => ({
  springs: [] as { toValue: number; done?: () => void }[],
  stopAnimation: vi.fn(),
}));
vi.mock("react-native", () => ({
  Animated: {
    Value: class {
      v: number;
      constructor(v: number) {
        this.v = v;
      }
      stopAnimation = anim.stopAnimation;
    },
    spring: (_v: unknown, cfg: { toValue: number }) => ({
      start: (done?: () => void) => {
        anim.springs.push({ toValue: cfg.toValue, done });
      },
    }),
  },
}));

import { DrawerProvider, useDrawer } from "./DrawerProvider";

function Probe() {
  const d = useDrawer();
  return (
    <div>
      <span data-testid="state">{d.isOpen ? "open" : "closed"}</span>
      <button onClick={d.open}>open</button>
      <button onClick={d.close}>close</button>
      <button onClick={d.toggle}>toggle</button>
    </div>
  );
}

beforeEach(() => {
  anim.springs.length = 0;
  anim.stopAnimation.mockClear();
});

// Opening flips isOpen at once (the drawer must mount before it slides in),
// closing keeps it open until the slide-out spring has finished (otherwise
// the drawer vanishes mid-motion), toggle follows the current state, an
// unmount stops any spring still in flight, and the hook refuses to run
// outside its provider.
describe("DrawerProvider", () => {
  it("opens immediately and closes only once the spring settles", () => {
    render(<DrawerProvider><Probe /></DrawerProvider>);
    expect(screen.getByTestId("state").textContent).toBe("closed");

    fireEvent.click(screen.getByText("open"));
    expect(screen.getByTestId("state").textContent).toBe("open");
    expect(anim.springs.at(-1)?.toValue).toBe(0);

    fireEvent.click(screen.getByText("close"));
    expect(anim.springs.at(-1)?.toValue).toBe(-280);
    expect(screen.getByTestId("state").textContent).toBe("open");
    act(() => anim.springs.at(-1)!.done!());
    expect(screen.getByTestId("state").textContent).toBe("closed");
  });

  it("toggle follows the current state", () => {
    render(<DrawerProvider><Probe /></DrawerProvider>);
    fireEvent.click(screen.getByText("toggle"));
    expect(screen.getByTestId("state").textContent).toBe("open");
    fireEvent.click(screen.getByText("toggle"));
    expect(anim.springs.at(-1)?.toValue).toBe(-280);
    act(() => anim.springs.at(-1)!.done!());
    expect(screen.getByTestId("state").textContent).toBe("closed");
  });

  it("stops an in-flight spring on unmount", () => {
    const { unmount } = render(<DrawerProvider><Probe /></DrawerProvider>);
    fireEvent.click(screen.getByText("open"));
    unmount();
    expect(anim.stopAnimation).toHaveBeenCalledTimes(1);
  });

  it("useDrawer throws outside the provider", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    expect(() => render(<Probe />)).toThrow(/within DrawerProvider/);
    spy.mockRestore();
  });
});
