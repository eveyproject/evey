import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { BackgroundJobsCard } from "./BackgroundJobsCard";

const setMutate = vi.fn();
const deleteMutate = vi.fn();
vi.mock("./hooks", () => ({
  useSetConfigEntry: () => ({ mutate: setMutate, isPending: false }),
  useDeleteConfigEntry: () => ({ mutate: deleteMutate, isPending: false }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

beforeEach(() => vi.clearAllMocks());

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

  it("saves every pace field at once, deleting the blank ones, and stays dirty until all writes land", () => {
    render(<BackgroundJobsCard entries={[{ key: "transcoding.level_backfill_batch", value: "50" }]} />);
    const save = screen.getByRole("button", { name: "common.save" }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    const batch = screen.getByLabelText("admin.serverConfig.backgroundJobs.levelBackfillBatchLabel") as HTMLInputElement;
    expect(batch.value).toBe("50");
    fireEvent.change(batch, { target: { value: "25" } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    expect(setMutate).toHaveBeenCalledWith({ key: "transcoding.level_backfill_batch", value: "25" }, expect.anything());
    expect(deleteMutate).toHaveBeenCalledTimes(5);
    expect(deleteMutate).toHaveBeenCalledWith("transcoding.keyframe_backfill_idle_sec", expect.anything());

    // Settle five of six writes: still dirty. The sixth lands: clean.
    const callbacks = [...setMutate.mock.calls.map((c) => c[1]), ...deleteMutate.mock.calls.map((c) => c[1])] as { onSuccess: () => void; onError: () => void }[];
    act(() => callbacks.slice(0, 5).forEach((cb) => cb.onSuccess()));
    expect(save.disabled).toBe(false);
    act(() => callbacks[5].onSuccess());
    expect(save.disabled).toBe(true);
  });

  it("a failed write keeps the form dirty so the edit can be retried", () => {
    render(<BackgroundJobsCard entries={[]} />);
    const batch = screen.getByLabelText("admin.serverConfig.backgroundJobs.levelBackfillBatchLabel");
    fireEvent.change(batch, { target: { value: "10" } });
    const save = screen.getByRole("button", { name: "common.save" }) as HTMLButtonElement;
    fireEvent.click(save);
    const callbacks = [...setMutate.mock.calls.map((c) => c[1]), ...deleteMutate.mock.calls.map((c) => c[1])] as { onSuccess: () => void; onError: () => void }[];
    act(() => {
      callbacks[0].onError();
      callbacks.slice(1).forEach((cb) => cb.onSuccess());
    });
    expect(save.disabled).toBe(false);
  });
});
