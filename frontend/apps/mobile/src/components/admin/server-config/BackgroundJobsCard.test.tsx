import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", async () => (await import("../../../test/rnDom")).rnDom);
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, o?: unknown) => {
      if (typeof o === "string") return o;
      if (o && typeof o === "object" && "label" in o) return `${key}:${String((o as { label: unknown }).label)}`;
      return key;
    },
  }),
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: new Proxy({}, { get: () => "#000" }), radii: { sm: 4, md: 8, lg: 12 } }),
}));

import { BackgroundJobsCard } from "./BackgroundJobsCard";
import { BACKGROUND_JOB_RATES, BACKGROUND_JOB_TOGGLES } from "@evey/client";

const infoButtons = (root: HTMLElement) => root.querySelectorAll('[data-testid="setting-info-button"]');

const saveEntry = vi.fn(async (_vars: { key: string; value: string }) => ({}));
const deleteEntry = vi.fn(async (_key: string) => ({}));
const quietSave = vi.fn(async (_vars: { key: string; value: string }) => ({}));
const quietDelete = vi.fn(async (_key: string) => ({}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));

beforeEach(() => {
  vi.clearAllMocks();
});

describe("BackgroundJobsCard", () => {
  it("turning a worker off writes the sentinel 'off' value", () => {
    render(<BackgroundJobsCard entries={[]} saveEntry={saveEntry} deleteEntry={deleteEntry} quietSaveEntry={quietSave} quietDeleteEntry={quietDelete} saving={false} />);
    fireEvent.click(screen.getByTestId("bg-toggle-transcoding.level_backfill"));
    expect(saveEntry).toHaveBeenCalledWith({ key: "transcoding.level_backfill", value: "off" });
  });

  it("turning a worker back on deletes the entry to revert to the server default", () => {
    render(
      <BackgroundJobsCard
        entries={[{ key: "transcoding.level_backfill", value: "off" }]}
        saveEntry={saveEntry}
        deleteEntry={deleteEntry}
        quietSaveEntry={quietSave}
        quietDeleteEntry={quietDelete}
        saving={false}
      />,
    );
    fireEvent.click(screen.getByTestId("bg-toggle-transcoding.level_backfill"));
    expect(deleteEntry).toHaveBeenCalledWith("transcoding.level_backfill");
    expect(saveEntry).not.toHaveBeenCalled();
  });

  it("saving pacing writes filled fields and deletes cleared ones, through the quiet writers, with one toast", async () => {
    render(
      <BackgroundJobsCard
        entries={[{ key: "transcoding.level_backfill_batch", value: "10" }]}
        saveEntry={saveEntry}
        deleteEntry={deleteEntry}
        quietSaveEntry={quietSave}
        quietDeleteEntry={quietDelete}
        saving={false}
      />,
    );
    fireEvent.change(screen.getByTestId("bg-rate-transcoding.level_backfill_batch"), { target: { value: "25" } });
    fireEvent.click(screen.getByTestId("bg-rates-save"));

    expect(quietSave).toHaveBeenCalledWith({ key: "transcoding.level_backfill_batch", value: "25" });
    // The five untouched (empty) fields revert to the server default.
    expect(quietDelete).toHaveBeenCalledWith("transcoding.keyframe_backfill_batch");
    expect(saveEntry).not.toHaveBeenCalled();
    expect(deleteEntry).not.toHaveBeenCalled();
    await waitFor(() => expect(toast.success).toHaveBeenCalledTimes(1));
  });

  it("save stays disabled until a pacing field is edited", () => {
    render(<BackgroundJobsCard entries={[]} saveEntry={saveEntry} deleteEntry={deleteEntry} quietSaveEntry={quietSave} quietDeleteEntry={quietDelete} saving={false} />);
    fireEvent.click(screen.getByTestId("bg-rates-save"));
    expect(quietSave).not.toHaveBeenCalled();
    expect(quietDelete).not.toHaveBeenCalled();
  });

  it("gives every toggle and pacing row exactly one 'i' button and a restart badge", () => {
    const { container } = render(<BackgroundJobsCard entries={[]} saveEntry={saveEntry} deleteEntry={deleteEntry} quietSaveEntry={quietSave} quietDeleteEntry={quietDelete} saving={false} />);
    const rows = BACKGROUND_JOB_TOGGLES.length + BACKGROUND_JOB_RATES.length;
    expect(infoButtons(container).length).toBe(rows);
    expect(container.querySelectorAll('[data-testid="setting-restart-badge"]').length).toBe(rows);
  });

  it("explains each pacing field by what it measures", () => {
    render(<BackgroundJobsCard entries={[]} saveEntry={saveEntry} deleteEntry={deleteEntry} quietSaveEntry={quietSave} quietDeleteEntry={quietDelete} saving={false} />);
    const batch = BACKGROUND_JOB_RATES.find((f) => f.key.endsWith("_batch"))!;
    fireEvent.click(screen.getByRole("button", { name: `admin.settingsLayout.moreInfo:${batch.labelKey}` }));
    expect(screen.getByRole("dialog").textContent).toContain("admin.settingsHelp.paceBatch");
  });
});
