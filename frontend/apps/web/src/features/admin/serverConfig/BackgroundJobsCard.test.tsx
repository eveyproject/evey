import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BackgroundJobsCard } from "./BackgroundJobsCard";

// Each write is a promise the test settles by hand.
type Pending = { resolve: (v: unknown) => void; reject: (e: unknown) => void };
const pending: Pending[] = [];
const deferred = () => new Promise((resolve, reject) => pending.push({ resolve, reject }));
const setMutate = vi.fn(deferred);
const deleteMutate = vi.fn(deferred);
vi.mock("./hooks", () => ({
  useSetConfigEntry: () => ({ mutateAsync: setMutate, isPending: false }),
  useDeleteConfigEntry: () => ({ mutateAsync: deleteMutate, isPending: false }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));

beforeEach(() => {
  vi.clearAllMocks();
  pending.length = 0;
});

// A boot-time flag reads as on unless its row says "off"; switching it off
// writes "off", switching it back on DELETES the row (server default), never
// writes "on". The pace fields save every field at once - a blank one deletes
// its row - and the form only stops being dirty once every write landed.
describe("BackgroundJobsCard", () => {
  it("toggles a flag by writing off or deleting the override", () => {
    render(<BackgroundJobsCard entries={[{ key: "transcoding.level_backfill", value: "off" }]} />);
    const pause = screen.getByRole("switch", { name: "admin.serverConfig.backgroundJobs.pauseBgOnPlaybackLabel" });
    const level = screen.getByRole("switch", { name: "admin.serverConfig.backgroundJobs.levelBackfillLabel" });
    expect(pause.getAttribute("aria-checked")).toBe("true");
    expect(level.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(pause);
    expect(setMutate).toHaveBeenCalledWith({ key: "transcoding.pause_bg_on_playback", value: "off" });
    fireEvent.click(level);
    expect(deleteMutate).toHaveBeenCalledWith("transcoding.level_backfill");
    expect(setMutate).toHaveBeenCalledTimes(1);
  });

  it("saves every pace field at once, deleting the blank ones, and stays dirty until all writes land", async () => {
    render(<BackgroundJobsCard entries={[{ key: "transcoding.level_backfill_batch", value: "50" }]} />);
    const save = screen.getByRole("button", { name: "common.save" }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    const batch = screen.getByLabelText("admin.serverConfig.backgroundJobs.levelBackfillBatchLabel") as HTMLInputElement;
    expect(batch.value).toBe("50");
    fireEvent.change(batch, { target: { value: "25" } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    expect(setMutate).toHaveBeenCalledWith({ key: "transcoding.level_backfill_batch", value: "25" });
    expect(deleteMutate).toHaveBeenCalledTimes(5);
    expect(deleteMutate).toHaveBeenCalledWith("transcoding.keyframe_backfill_idle_sec");

    // Settle five of six writes: still dirty. The sixth lands: clean.
    pending.slice(0, 5).forEach((p) => p.resolve({}));
    await Promise.resolve();
    expect(save.disabled).toBe(false);
    expect(toast.success).not.toHaveBeenCalled();
    pending[5].resolve({});
    await waitFor(() => expect(save.disabled).toBe(true));
    // Six writes, one toast.
    expect(toast.success).toHaveBeenCalledTimes(1);
    expect(toast.success).toHaveBeenCalledWith("admin.serverConfig.configUpdated");
  });

  it("a failed write keeps the form dirty so the edit can be retried", async () => {
    render(<BackgroundJobsCard entries={[]} />);
    const batch = screen.getByLabelText("admin.serverConfig.backgroundJobs.levelBackfillBatchLabel");
    fireEvent.change(batch, { target: { value: "10" } });
    const save = screen.getByRole("button", { name: "common.save" }) as HTMLButtonElement;
    fireEvent.click(save);
    pending[0].reject(new Error("boom"));
    pending.slice(1).forEach((p) => p.resolve({}));
    await waitFor(() => expect(toast.error).toHaveBeenCalledTimes(1));
    expect(toast.error).toHaveBeenCalledWith("admin.serverConfig.failedUpdate");
    expect(toast.success).not.toHaveBeenCalled();
    expect(save.disabled).toBe(false);
  });
});
