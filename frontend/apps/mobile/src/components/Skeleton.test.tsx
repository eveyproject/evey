import { describe, it, expect, vi } from "vitest";
import { render } from "@testing-library/react";
import React from "react";

// The shimmer is one looping fade started on mount and STOPPED on unmount:
// a list of skeletons that kept animating after being replaced by real rows
// would keep the JS thread busy for nothing.
const anim = vi.hoisted(() => ({ started: 0, stopped: 0 }));
vi.mock("react-native", () => ({
  Animated: {
    Value: class { constructor(public v: number) {} },
    timing: () => ({}),
    sequence: () => ({}),
    loop: () => ({ start: () => { anim.started++; }, stop: () => { anim.stopped++; } }),
    View: ({ style }: any) => {
      const flat = Object.assign({}, ...(Array.isArray(style) ? style : [style]).filter(Boolean));
      return React.createElement("div", { "data-testid": "skeleton", "data-w": flat.width, "data-h": flat.height, "data-r": flat.borderRadius, "data-mt": flat.marginTop });
    },
  },
}));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { surface: "#111" } }) }));

import { Skeleton } from "./Skeleton";

describe("Skeleton (mobile)", () => {
  it("sizes the placeholder, merges extra style, and stops its loop on unmount", () => {
    const { getByTestId, unmount } = render(<Skeleton width={120} height={16} borderRadius={6} style={{ marginTop: 6 }} />);
    const el = getByTestId("skeleton");
    expect(el.getAttribute("data-w")).toBe("120");
    expect(el.getAttribute("data-h")).toBe("16");
    expect(el.getAttribute("data-r")).toBe("6");
    expect(el.getAttribute("data-mt")).toBe("6");
    expect(anim.started).toBe(1);
    expect(anim.stopped).toBe(0);
    unmount();
    expect(anim.stopped).toBe(1);
  });

  it("defaults the radius to 4", () => {
    const { getByTestId } = render(<Skeleton width="100%" height={64} />);
    expect(getByTestId("skeleton").getAttribute("data-r")).toBe("4");
    expect(getByTestId("skeleton").getAttribute("data-w")).toBe("100%");
  });
});
