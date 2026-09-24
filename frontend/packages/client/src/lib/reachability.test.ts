import { afterEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryObserver, onlineManager } from "@tanstack/react-query";
import { setupReachability } from "./reachability";

// A real QueryClient: what matters is what React Query DOES once told the
// server is gone - pause instead of failing, and come back on its own.

const noAnswer = () => Object.assign(new Error("Network request failed"), {});
const flush = () => new Promise((r) => setTimeout(r, 0));

function harness() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  // What QueryClientProvider does: subscribes the client to the online state.
  queryClient.mount();
  mounted.push(queryClient);
  let reachable = true;
  const probe = vi.fn(async () => reachable);
  let foreground: () => void = () => {};
  setupReachability({
    queryClient,
    probe,
    onForeground: (l) => { foreground = l; return () => {}; },
    pollMs: 50,
  });
  return {
    queryClient,
    probe,
    setReachable: (v: boolean) => { reachable = v; },
    wake: () => foreground(),
  };
}

const mounted: QueryClient[] = [];

afterEach(() => {
  mounted.splice(0).forEach((c) => c.unmount());
  onlineManager.setEventListener(() => undefined);
  onlineManager.setOnline(true);
});

describe("setupReachability", () => {
  it("goes offline when a request gets no answer and the server does not either, and back when it does", async () => {
    const h = harness();
    h.setReachable(false);
    let calls = 0;
    const fetchFn = vi.fn(async () => { calls++; if (calls === 1) throw noAnswer(); return "home"; });
    const observer = new QueryObserver(h.queryClient, { queryKey: ["home"], queryFn: fetchFn });
    const unsubscribe = observer.subscribe(() => {});

    await vi.waitFor(() => expect(onlineManager.isOnline()).toBe(false));

    // Offline, a new fetch waits instead of failing.
    void h.queryClient.refetchQueries({ queryKey: ["home"] });
    await flush();
    expect(observer.getCurrentResult().fetchStatus).toBe("paused");

    // The server answers again: the poll notices, the query resumes.
    h.setReachable(true);
    await vi.waitFor(() => expect(observer.getCurrentResult().data).toBe("home"));
    expect(onlineManager.isOnline()).toBe(true);
    unsubscribe();
  });

  it("stays online when only the request failed (the server still answers)", async () => {
    const h = harness();
    const observer = new QueryObserver(h.queryClient, { queryKey: ["x"], queryFn: async () => { throw noAnswer(); } });
    const unsubscribe = observer.subscribe(() => {});
    await vi.waitFor(() => expect(h.probe).toHaveBeenCalled());
    await flush();
    expect(onlineManager.isOnline()).toBe(true);
    unsubscribe();
  });

  it("does not probe on an answer the server gave (an HTTP error is not a lost network)", async () => {
    const h = harness();
    const observer = new QueryObserver(h.queryClient, {
      queryKey: ["y"],
      queryFn: async () => { throw Object.assign(new Error("boom"), { httpStatus: 500 }); },
    });
    const unsubscribe = observer.subscribe(() => {});
    await vi.waitFor(() => expect(observer.getCurrentResult().status).toBe("error"));
    expect(h.probe).not.toHaveBeenCalled();
    unsubscribe();
  });

  // The box wakes from standby: the network came back while it slept, so no
  // state changed and the failed screen would stay failed without this.
  it("refetches the failed screens when the app comes back to the foreground", async () => {
    const h = harness();
    let fail = true;
    const observer = new QueryObserver(h.queryClient, {
      queryKey: ["z"],
      queryFn: async () => { if (fail) throw Object.assign(new Error("boom"), { httpStatus: 502 }); return "ok"; },
    });
    const unsubscribe = observer.subscribe(() => {});
    await vi.waitFor(() => expect(observer.getCurrentResult().status).toBe("error"));

    fail = false;
    h.wake();
    await vi.waitFor(() => expect(observer.getCurrentResult().data).toBe("ok"));
    unsubscribe();
  });
});
