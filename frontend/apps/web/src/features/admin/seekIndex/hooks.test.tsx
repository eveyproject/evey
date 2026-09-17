import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

// The generated query/mutation hooks are stubbed; the triage helpers stay
// real so the breakdown is computed the way the page computes it.
const srv = vi.hoisted(() => ({
  job: undefined as unknown,
  jobOpts: null as any,
  scan: undefined as unknown,
  mutations: {} as Record<string, any>,
}));
vi.mock("@evey/api", async (importOriginal) => {
  const real = await importOriginal<Record<string, unknown>>();
  const mutation = (name: string) => (o: any) => {
    srv.mutations[name] = o.mutation;
    return { mutate: vi.fn(), isPending: false, mutation: o.mutation };
  };
  return {
    ...real,
    useLibraryServiceListLibraries: () => ({ data: { libraries: [{ id: "l1" }] } }),
    useAdminMediaServiceGetSeekIndexJob: (o: any) => {
      srv.jobOpts = o.query;
      return { data: srv.job, queryKey: ["job"] };
    },
    useAdminMediaServiceScanSeekIndex: () => ({ data: srv.scan }),
    useAdminMediaServiceStartSeekIndexScan: mutation("startScan"),
    useAdminMediaServiceCancelSeekIndexJob: mutation("cancelJob"),
    useAdminMediaServiceReindexMediaSeekIndex: mutation("repairOne"),
    useAdminMediaServiceRollbackMediaSeekIndex: mutation("rollbackOne"),
    useAdminMediaServiceReindexLibrarySeekIndex: mutation("repairLib"),
    useAdminMediaServiceRollbackLibrarySeekIndex: mutation("rollbackLib"),
    useAdminMediaServiceDeleteSeekIndexBackup: mutation("deleteBackup"),
    useAdminMediaServicePurgeSeekIndexBackups: mutation("purgeLib"),
    useAdminMediaServiceAckSeekIndexMedia: mutation("ackOne"),
    useAdminMediaServiceAckSeekIndexGroup: mutation("ackGroup"),
    useAdminMediaServiceFixContainerMedia: mutation("fixContainerOne"),
    useAdminMediaServiceReinterleaveMedia: mutation("reinterleaveOne"),
  };
});
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}:${Object.values(o).join(",")}` : k) }),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));

import { useSeekIndexPage, SEEK_INDEX_PAGE_SIZE } from "./hooks";

const row = (id: string, over: Record<string, unknown> = {}) => ({
  mediaId: id, title: id, path: `/m/${id}.mkv`, container: "mkv", sizeBytes: "10", hasIndex: true, ...over,
});

beforeEach(() => {
  vi.clearAllMocks();
  srv.job = { running: false };
  srv.scan = undefined;
});

// Completion is observed from the job heartbeat, not from a mutation
// callback: the scan cache is refreshed and the outcome named honestly - an
// error, then failures, then a run that mostly skipped (never a plain
// success for that one), then a real success.
describe("useSeekIndexPage job completion", () => {
  function finishWith(last: Record<string, unknown>) {
    srv.job = { running: true };
    const h = renderHook(() => useSeekIndexPage());
    srv.job = { running: false, last };
    h.rerender();
    return h;
  }

  it("polls fast while a job runs and slowly otherwise", () => {
    renderHook(() => useSeekIndexPage());
    expect(srv.jobOpts.refetchInterval({ state: { data: { running: true } } })).toBe(1500);
    expect(srv.jobOpts.refetchInterval({ state: { data: { running: false } } })).toBe(10_000);
    expect(srv.jobOpts.refetchIntervalInBackground).toBe(false);
  });

  it("names an aborted run by its error", () => {
    finishWith({ kind: "scan", error: "disk gone" });
    expect(toast.error).toHaveBeenCalledWith("admin.seekIndex.lastJobError:disk gone");
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["/api/v1/admin/media/seek-index"] });
  });

  it("reports failures before anything else", () => {
    finishWith({ kind: "reindex", done: 10, failed: 2, skipped: 5 });
    expect(toast.error).toHaveBeenCalledWith("admin.seekIndex.jobCompletedFailures:admin.seekIndex.jobReindex,10,2");
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("a run that skipped says how many were really done", () => {
    finishWith({ kind: "reindex", done: 138, failed: 0, skipped: 133 });
    expect(toast.success).toHaveBeenCalledWith("admin.seekIndex.jobCompletedSkipped:admin.seekIndex.jobReindex,5,133");
  });

  it("a clean run is a plain success, and nothing fires without a transition", () => {
    finishWith({ kind: "purge", done: 4, failed: 0 });
    expect(toast.success).toHaveBeenCalledWith("admin.seekIndex.jobCompleted:admin.seekIndex.jobPurge,4");
    toast.success.mockClear();
    const h = renderHook(() => useSeekIndexPage());
    h.rerender();
    expect(toast.success).not.toHaveBeenCalled();
  });
});

// The rows the table shows are the rows the cards count: the breakdown keeps
// files a repair already refused apart from the damaged byte total, a new
// view or search returns to page one, and the page never points past the
// last one.
describe("useSeekIndexPage triage", () => {
  it("breaks the scan down by defect and keeps resync-required apart", () => {
    srv.scan = {
      scannedAtMs: "1700000000000",
      withBackup: 2,
      items: [
        row("missing", { hasIndex: false }),
        row("damaged", { damagedBytes: "4096" }),
        row("refused", { resyncRequired: true, damagedBytes: "8192" }),
        row("wrongc", { wrongContainer: true }),
        row("coarse", { seekCoarse: true }),
        row("trunc", { truncated: true, seekCoarse: true }),
        row("acked", { truncated: true, ackedAtMs: "1" }),
        row("ok"),
      ],
      volumes: [{ missingFiles: 2, missingBytes: "100", freeBytes: "1000" }, { missingFiles: 0 }],
    };
    const { result } = renderHook(() => useSeekIndexPage());
    const s = result.current;
    expect(s.breakdown).toMatchObject({ missing: 1, damaged: 2, damagedBytes: 12288, resyncRequired: 1, wrongContainer: 1, seekCoarse: 1, truncated: 1 });
    expect(s.triage.repairable).toBe(4);
    expect(s.withBackup).toBe(2);
    expect(s.repairVolumes).toHaveLength(1);
    expect(s.counts.all).toBe(8);
    expect(s.libraries).toEqual([{ id: "l1" }]);
  });

  it("treats a scan without a timestamp as no scan", () => {
    srv.scan = { scannedAtMs: "0", items: [row("x")] };
    const { result } = renderHook(() => useSeekIndexPage());
    expect(result.current.result).toBeUndefined();
    expect(result.current.items).toEqual([]);
  });

  it("paginates the visible rows, resets to page one on a new view or search, and clamps", () => {
    const items = Array.from({ length: SEEK_INDEX_PAGE_SIZE + 5 }, (_, i) => row(`f${String(i).padStart(3, "0")}`, { hasIndex: false }));
    srv.scan = { scannedAtMs: "1", items };
    const { result } = renderHook(() => useSeekIndexPage());
    expect(result.current.totalPages).toBe(2);
    act(() => result.current.setPage(2));
    expect(result.current.page).toBe(2);
    expect(result.current.paged).toHaveLength(5);

    act(() => result.current.setQuery("f00"));
    expect(result.current.page).toBe(1);
    expect(result.current.visible).toHaveLength(10);

    act(() => result.current.setQuery(""));
    act(() => result.current.setPage(2));
    act(() => result.current.setView("all"));
    expect(result.current.page).toBe(1);

    act(() => result.current.setPage(9));
    expect(result.current.page).toBe(2);
  });

  it("start feedback: a started job is announced and the heartbeat refreshed; a refusal is an error", () => {
    renderHook(() => useSeekIndexPage());
    srv.mutations.startScan.onSuccess();
    expect(toast.info).toHaveBeenCalledWith("admin.seekIndex.scanStarted");
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["job"] });
    srv.mutations.repairOne.onError({ message: "locked" });
    expect(toast.error).toHaveBeenCalledWith("locked");
    srv.mutations.ackOne.onSuccess({}, { data: { acked: false } });
    expect(toast.success).toHaveBeenCalledWith("admin.seekIndex.unackDone");
    srv.mutations.ackGroup.onSuccess({ acked: 3 });
    expect(toast.success).toHaveBeenCalledWith("admin.seekIndex.ackGroupDone:3");
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["/api/v1/admin/media/seek-index"] });
  });
});
