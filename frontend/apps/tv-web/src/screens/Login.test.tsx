import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";

// "Remember me": opted in, the username comes back the next time the viewer
// lands here; the password is never kept in the browser's storage.
const { mutate, replaceMock, setRefreshToken } = vi.hoisted(() => ({
  mutate: vi.fn(),
  replaceMock: vi.fn(),
  setRefreshToken: vi.fn(),
}));

vi.mock("../tv/focus", () => ({
  useArrivalFocus: () => {},
  useBackHandler: () => {},
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, f?: unknown) => (typeof f === "string" ? f : k) }) }));
vi.mock("../app/router", () => ({ replace: (...a: unknown[]) => replaceMock(...a), navigate: vi.fn() }));
vi.mock("../lib/session", () => ({ refreshTokenStore: { setRefreshToken } }));
const authState = { setTokens: vi.fn() };
vi.mock("@evey/store", () => ({ useAuthStore: (sel: any) => sel(authState) }));
let onSuccess: (data: unknown) => Promise<void> = async () => {};
vi.mock("@evey/api", () => ({
  useUserServiceLogin: (opts: { mutation: { onSuccess: (d: unknown) => Promise<void> } }) => {
    onSuccess = opts.mutation.onSuccess;
    return { mutate, isPending: false };
  },
}));

import { LoginScreen } from "./Login";

const fields = () => screen.getAllByRole("textbox").concat([...document.querySelectorAll("input[type=password]")] as HTMLElement[]) as HTMLInputElement[];

beforeEach(() => {
  localStorage.clear();
  mutate.mockReset();
});

describe("tv-web login, remember me", () => {
  it("keeps the credentials when the viewer opted in", async () => {
    render(<LoginScreen />);
    const [user, pass] = fields();
    fireEvent.change(user, { target: { value: "alice" } });
    fireEvent.change(pass, { target: { value: "s3cret" } });
    fireEvent.click(screen.getByRole("button", { name: "Remember me" }));
    fireEvent.click(screen.getAllByRole("button", { name: "auth.login.signIn" })[0]);
    expect(mutate).toHaveBeenCalledWith({ data: { username: "alice", password: "s3cret" } });
    await onSuccess({ accessToken: "a", refreshToken: "r" });
    expect(localStorage.getItem("evey-tv-saved-username")).toBe("alice");
    expect(localStorage.getItem("evey-tv-saved-password")).toBeNull();
    expect(JSON.stringify({ ...localStorage })).not.toContain("s3cret");
  });

  it("fills the username back in, never a password", () => {
    localStorage.setItem("evey-tv-saved-username", "alice");
    render(<LoginScreen />);
    const [user, pass] = fields();
    expect(user.value).toBe("alice");
    expect(pass.value).toBe("");
    expect(screen.getByRole("button", { name: "Remember me" }).className).toContain("active");
  });

  it("erases a password kept by an earlier version without using it", () => {
    localStorage.setItem("evey-tv-saved-username", "alice");
    localStorage.setItem("evey-tv-saved-password", "s3cret");
    render(<LoginScreen />);
    const [user, pass] = fields();
    expect(user.value).toBe("alice");
    expect(pass.value).toBe("");
    expect(localStorage.getItem("evey-tv-saved-password")).toBeNull();
  });

  it("forgets the username when the viewer signs in with the box unticked", async () => {
    localStorage.setItem("evey-tv-saved-username", "alice");
    render(<LoginScreen />);
    const [, pass] = fields();
    fireEvent.change(pass, { target: { value: "s3cret" } });
    fireEvent.click(screen.getByRole("button", { name: "Remember me" }));
    fireEvent.click(screen.getAllByRole("button", { name: "auth.login.signIn" })[0]);
    await onSuccess({ accessToken: "a", refreshToken: "r" });
    expect(localStorage.getItem("evey-tv-saved-username")).toBeNull();
  });

  it("keeps nothing by default", async () => {
    render(<LoginScreen />);
    const [user, pass] = fields();
    fireEvent.change(user, { target: { value: "bob" } });
    fireEvent.change(pass, { target: { value: "pw" } });
    fireEvent.click(screen.getAllByRole("button", { name: "auth.login.signIn" })[0]);
    await onSuccess({ accessToken: "a" });
    expect(localStorage.getItem("evey-tv-saved-username")).toBeNull();
    expect(localStorage.getItem("evey-tv-saved-password")).toBeNull();
  });
});
