import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";
import { DemoReadOnly } from "./DemoReadOnly";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const demo = vi.hoisted(() => ({ on: false }));
vi.mock("@/hooks/useDemoMode", () => ({ useDemoMode: () => demo.on }));

beforeEach(() => {
  demo.on = false;
});

// Off the demo the wrapper is invisible; on the demo every control beneath it
// is disabled by the fieldset and the page says why.
describe("DemoReadOnly", () => {
  it("leaves the children untouched off the demo", () => {
    render(<DemoReadOnly><button>save</button></DemoReadOnly>);
    expect(screen.getByText("save")).not.toBeDisabled();
    expect(screen.queryByTestId("demo-readonly")).toBeNull();
  });

  it("disables every control and names the reason on the demo", () => {
    demo.on = true;
    render(
      <DemoReadOnly>
        <button>save</button>
        <input aria-label="cron" />
      </DemoReadOnly>,
    );
    expect(screen.getByText("save")).toBeDisabled();
    expect(screen.getByLabelText("cron")).toBeDisabled();
    expect(screen.getByTestId("demo-readonly")).toHaveTextContent("common.demo.readOnly");
  });
});
