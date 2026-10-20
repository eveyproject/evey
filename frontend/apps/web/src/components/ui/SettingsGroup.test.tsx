import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { SettingsGroup } from "./SettingsGroup";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

// The "i" sits beside the heading, not inside it: the heading's accessible
// name stays the title alone.
describe("SettingsGroup", () => {
  it("keeps the heading name clean and puts the 'i' next to it", () => {
    render(
      <SettingsGroup title="Cache" info="Why it matters" titleId="cache-title" footer={<button type="button">Save</button>}>
        <p>row</p>
      </SettingsGroup>,
    );
    const heading = screen.getByRole("heading", { name: "Cache" });
    expect(heading).toHaveAttribute("id", "cache-title");
    expect(heading.querySelector("[role=button]")).toBeNull();
    expect(screen.getByRole("button", { name: "admin.settingsLayout.moreInfo" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
  });
});
