import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";
import { DemoBanner } from "./DemoBanner";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const demo = vi.hoisted(() => ({ on: false }));
vi.mock("@/hooks/useDemoMode", () => ({ useDemoMode: () => demo.on }));

beforeEach(() => {
  demo.on = false;
});

// The banner exists only when the server says this is the demo, and then it
// says what the demo does to a visitor's changes.
describe("DemoBanner", () => {
  it("renders nothing off the demo", () => {
    const { container } = render(<DemoBanner />);
    expect(container.firstChild).toBeNull();
  });

  it("names the demo's boundary on the demo", () => {
    demo.on = true;
    render(<DemoBanner />);
    expect(screen.getByTestId("demo-banner")).toHaveTextContent("common.demo.banner");
  });
});
