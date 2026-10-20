import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { SettingRow } from "./SettingRow";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o?.label ? `${k}(${String(o.label)})` : k) }),
}));

// One layout for every setting: the label (a real <label> when the control
// has an id), its "i" explanation, the restart badge, a hint line and the
// default/reset line. Each part appears only when asked for.
describe("SettingRow", () => {
  it("renders the label for its control, the 'i', the hint and the restart badge", () => {
    render(
      <SettingRow label="Server name" htmlFor="name" info="What the apps show." hint="Short hint" restart>
        <input id="name" />
      </SettingRow>,
    );
    expect(screen.getByLabelText("Server name")).toHaveAttribute("id", "name");
    expect(screen.getByText("Short hint")).toBeInTheDocument();
    expect(screen.getByTestId("setting-restart-badge")).toHaveTextContent("admin.serverConfig.appliedOnRestart");
    const info = screen.getByRole("button", { name: "admin.settingsLayout.moreInfo(Server name)" });
    fireEvent.click(info);
    expect(screen.getByRole("dialog")).toHaveTextContent("What the apps show.");
  });

  it("shows nothing optional when nothing optional is given", () => {
    render(
      <SettingRow label="Bare">
        <input aria-label="bare control" />
      </SettingRow>,
    );
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByTestId("setting-restart-badge")).toBeNull();
    expect(screen.queryByTestId("setting-using-default")).toBeNull();
  });

  it("says which default applies, and offers a reset once a value is chosen", () => {
    const onReset = vi.fn();
    const { rerender } = render(
      <SettingRow label="Rate" reset={{ overridden: false, defaultText: "10", onReset }}>
        <input aria-label="rate" />
      </SettingRow>,
    );
    expect(screen.getByTestId("setting-using-default")).toHaveTextContent("admin.serverSettings.usingDefault 10");
    expect(screen.queryByRole("button", { name: "admin.serverSettings.reset" })).toBeNull();

    rerender(
      <SettingRow label="Rate" reset={{ overridden: true, defaultText: "10", onReset }}>
        <input aria-label="rate" />
      </SettingRow>,
    );
    fireEvent.click(screen.getByRole("button", { name: "admin.serverSettings.reset" }));
    expect(onReset).toHaveBeenCalledOnce();
  });

  it("lays a wide control under the label and a field beside it", () => {
    const { container, rerender } = render(
      <SettingRow label="Tokens" kind="wide" data-testid="row">
        <textarea aria-label="tokens" />
      </SettingRow>,
    );
    expect(screen.getByTestId("row").className).not.toContain("sm:flex-row");
    rerender(
      <SettingRow label="Tokens" kind="field" data-testid="row">
        <input aria-label="tokens" />
      </SettingRow>,
    );
    expect(screen.getByTestId("row").className).toContain("sm:flex-row");
    expect(within(container).getByLabelText("tokens").parentElement?.className).toContain("sm:w-72");
  });
});
