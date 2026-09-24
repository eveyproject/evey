import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

const { navigateMock } = vi.hoisted(() => ({ navigateMock: vi.fn() }));

vi.mock("../tv/focus", () => ({
  useArrivalFocus: () => {},
  useBackHandler: () => {},
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../app/router", () => ({ navigate: (...a: unknown[]) => navigateMock(...a), replace: vi.fn(), goBack: vi.fn() }));
vi.mock("./SearchPanel", () => ({ SearchPanel: () => <div data-testid="search-panel" /> }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useNotificationServiceGetUnreadCount: () => ({ data: { count: 3 }, isError: false }),
}));
vi.mock("@evey/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/client")>()),
  useActiveProfile: () => ({ profile: { id: "prf-1", name: "Gravity" } }),
}));

import { TabShell, TAB_ROUTES } from "./Shell";
import { HOME_CUSTOMIZE_EVENT } from "./StackHeader";

beforeEach(() => navigateMock.mockReset());

// The Android television's frame: the rail on the left, the header with the
// search field, the bell and the viewer's own face.
describe("the tab frame", () => {
  it("walks the rail to each tab", () => {
    render(<TabShell current="home"><div /></TabShell>);
    fireEvent.click(screen.getByLabelText("common.nav.libraries"));
    expect(navigateMock).toHaveBeenCalledWith({ name: "libraries" });
  });

  it("names the profile in front of the viewer and opens the picker on it", () => {
    render(<TabShell current="home"><div /></TabShell>);
    expect(screen.getByText("G")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("mobile.settings.switchProfile"));
    expect(navigateMock).toHaveBeenCalledWith({ name: "profiles", pick: true });
  });

  it("shows the unread count and opens the notifications", () => {
    render(<TabShell current="home"><div /></TabShell>);
    expect(screen.getByText("3")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("common.notifications.title"));
    expect(navigateMock).toHaveBeenCalledWith({ name: "notifications" });
  });

  it("offers Customize on the home only, and hands it to the home", () => {
    const heard = vi.fn();
    window.addEventListener(HOME_CUSTOMIZE_EVENT, heard);
    const { unmount } = render(<TabShell current="home"><div /></TabShell>);
    fireEvent.click(screen.getByLabelText("common.feeds.customize"));
    expect(heard).toHaveBeenCalled();
    unmount();
    render(<TabShell current="settings"><div /></TabShell>);
    expect(screen.queryByLabelText("common.feeds.customize")).toBeNull();
    window.removeEventListener(HOME_CUSTOMIZE_EVENT, heard);
  });

  it("opens the search over the screen instead of leaving it", () => {
    render(<TabShell current="home"><div /></TabShell>);
    fireEvent.click(screen.getByLabelText("common.nav.search"));
    expect(screen.getByTestId("search-panel")).toBeTruthy();
    expect(navigateMock).not.toHaveBeenCalled();
  });

  it("keeps the rail to the Android television's tabs", () => {
    expect([...TAB_ROUTES].sort()).toEqual(["favorites", "home", "libraries", "music", "requests", "search", "settings"]);
  });
});
