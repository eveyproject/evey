import { describe, expect, it, vi, beforeEach } from "vitest";
import { render, waitFor } from "@testing-library/react";

// The code is on the screen for the whole room; the secret only ever reaches
// this browser. The screen declares it will send it back, and does so on every
// poll, so a phone that scanned the QR cannot collect the tokens in its place.
const { checkMock, initiateArgs } = vi.hoisted(() => ({
  checkMock: vi.fn(),
  initiateArgs: [] as unknown[],
}));

vi.mock("../tv/focus", () => ({
  useArrivalFocus: () => {},
  useBackHandler: () => {},
  FocusRoot: ({ children, className }: any) => <div className={className}>{children}</div>,
  useFocusable: () => ({ ref: { current: null }, focused: false, focusSelf: () => {} }),
}));
vi.mock("../components/ui", () => ({
  Pressable: ({ children, onPress, ...p }: any) => <button onClick={onPress} {...p}>{children}</button>,
  Spinner: () => <div data-testid="spinner" />,
}));
vi.mock("react-qr-code", () => ({ default: ({ value }: { value: string }) => <div data-testid="qr" data-value={value} /> }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, f?: unknown) => (typeof f === "string" ? f : k) }) }));
vi.mock("../app/router", () => ({ replace: vi.fn(), goBack: vi.fn(), navigate: vi.fn() }));
vi.mock("../lib/session", () => ({ refreshTokenStore: { setRefreshToken: vi.fn(async () => undefined) } }));
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: any) => sel({ setTokens: vi.fn() }),
  useServerListStore: (sel: any) => sel({ getActiveServer: () => ({ url: "http://srv.test" }) }),
}));
vi.mock("@evey/api", () => ({
  useUserServiceInitiateQuickConnect: ({ mutation }: any) => ({
    mutate: (vars?: unknown) => {
      initiateArgs.push(vars);
      mutation.onSuccess({ code: "qc-code-abc123", secret: "qc-secret-1" });
    },
    isError: false,
  }),
  checkQuickConnect: (...a: unknown[]) => checkMock(...a),
}));

import { QuickConnectScreen } from "./QuickConnect";

beforeEach(() => {
  checkMock.mockReset().mockResolvedValue({ authorized: false });
  initiateArgs.length = 0;
});

describe("tv-web quick connect secret", () => {
  it("declares itself secret-aware and polls with the secret it was handed", async () => {
    render(<QuickConnectScreen />);
    await waitFor(() => expect(checkMock).toHaveBeenCalled());
    expect(initiateArgs[0]).toEqual({ params: { secretAware: true } });
    expect(checkMock).toHaveBeenCalledWith("qc-code-abc123", "qc-secret-1");
  });
});
