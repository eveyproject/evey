import React from "react";
import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider, useMutation } from "@tanstack/react-query";

// The net under every write: a mutation that fails without saying so leaves the
// person believing it worked. These pin the three cases that matter - it speaks
// when nobody else does, it stays quiet when the caller already handles the
// failure, and it stays quiet when the caller asked it to.

const errorToast = vi.fn();
vi.mock("@evey/store", () => ({ toast: { error: (m: string) => errorToast(m) } }));
vi.mock("./mutationErrorMessage", () => ({ mutationErrorMessage: () => "went wrong" }));

async function wrapperFor() {
  // The real net, not a copy of its rules: a test that reimplements them would
  // stay green while query-client.ts drifted.
  const { mutationErrorNet } = await import("./mutationErrorNet");
  const qc = new QueryClient({
    defaultOptions: { mutations: { retry: false }, queries: { retry: false } },
    mutationCache: mutationErrorNet(),
  });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

const boom = () => Promise.reject(new Error("nope"));

beforeEach(() => errorToast.mockClear());

describe("the tv mutation error net", () => {
  it("speaks when the caller handles nothing", async () => {
    const wrapper = await wrapperFor();
    const { result } = renderHook(() => useMutation({ mutationFn: boom }), { wrapper });
    result.current.mutate(undefined);
    await waitFor(() => expect(errorToast).toHaveBeenCalledWith("went wrong"));
  });

  it("stays quiet when the caller already reports the failure", async () => {
    const wrapper = await wrapperFor();
    const ownHandler = vi.fn();
    const { result } = renderHook(
      () => useMutation({ mutationFn: boom, onError: ownHandler }),
      { wrapper },
    );
    result.current.mutate(undefined);
    await waitFor(() => expect(ownHandler).toHaveBeenCalled());
    expect(errorToast).not.toHaveBeenCalled();
  });

  it("stays quiet for a mutation that asked to fail silently", async () => {
    const wrapper = await wrapperFor();
    const { result } = renderHook(
      () => useMutation({ mutationFn: boom, meta: { silentError: true } }),
      { wrapper },
    );
    result.current.mutate(undefined);
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(errorToast).not.toHaveBeenCalled();
  });
});
