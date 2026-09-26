import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { ShareLinkButton } from "./ShareLinkButton";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
// The mutation hook is replaced by a recorder that hands back the options so
// the test can fire the success/error paths with a chosen response.
const hook = vi.hoisted(() => ({ opts: null as any, mutate: vi.fn() }));
vi.mock("@evey/api", () => ({
  useShareServiceCreateShareLink: (o: any) => {
    hook.opts = o.mutation;
    return { mutate: hook.mutate, isPending: false };
  },
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, loading }: any) => React.createElement("button", { onClick, disabled: loading }, children),
}));

const writeText = vi.fn();
beforeEach(() => {
  vi.clearAllMocks();
  // navigator.clipboard is a prototype getter: define an own property over it.
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
  writeText.mockResolvedValue(undefined);
});
afterEach(() => {
  // @ts-expect-error test cleanup
  delete navigator.clipboard;
});

// A share link is minted for 48 h, made absolute against this origin and
// copied; a blocked clipboard still shows the URL instead of losing it; an
// empty response or a refusal is an error, with the server's message when
// there is one.
describe("ShareLinkButton", () => {
  it("mints a 48 h link and copies the absolute URL", async () => {
    render(<ShareLinkButton mediaId="m1" />);
    fireEvent.click(screen.getByText("media.library.shareLink"));
    expect(hook.mutate).toHaveBeenCalledWith({ mediaItemId: "m1", data: { durationHours: 48 } });

    await hook.opts.onSuccess({ url: "/s/abc" });
    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/s/abc`);
    expect(toast.success).toHaveBeenCalledWith("media.library.shareLinkCopied");
  });

  it("surfaces the URL when the clipboard is blocked", async () => {
    writeText.mockRejectedValueOnce(new Error("denied"));
    render(<ShareLinkButton mediaId="m1" />);
    await hook.opts.onSuccess({ url: "/s/abc" });
    await waitFor(() => expect(toast.info).toHaveBeenCalledWith(`${window.location.origin}/s/abc`));
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("an empty response or a refusal is an error", async () => {
    render(<ShareLinkButton mediaId="m1" />);
    await hook.opts.onSuccess({});
    expect(toast.error).toHaveBeenCalledWith("media.library.shareFailed");
    expect(writeText).not.toHaveBeenCalled();

    hook.opts.onError(new Error("forbidden"));
    expect(toast.error).toHaveBeenLastCalledWith("forbidden");
    hook.opts.onError(new Error(""));
    expect(toast.error).toHaveBeenLastCalledWith("media.library.shareFailed");
  });
});
