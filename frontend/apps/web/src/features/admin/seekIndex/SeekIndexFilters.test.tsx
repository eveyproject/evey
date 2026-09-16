import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { SeekIndexFilters } from "./SeekIndexFilters";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}:${Object.values(o).join(",")}` : k) }),
}));

function mount(over: Record<string, unknown> = {}) {
  const props = {
    view: "attention", setView: vi.fn(), query: "", setQuery: vi.fn(), sort: "priority", setSort: vi.fn(),
    counts: { attention: 7, repairable: 5, informational: 2, acked: 1, backups: 3, all: 40 },
    result: { scannedAtMs: "1700000000000" },
    ...over,
  };
  render(<SeekIndexFilters {...(props as any)} />);
  return props;
}

// One chip per view carrying its live count (so the bar says what each view
// hides), the active one marked; the search and sort report through their
// setters; the scan timestamp is rendered as a local date, not raw ms.
describe("SeekIndexFilters", () => {
  it("renders a counted chip per view and switches on click", () => {
    const p = mount();
    const chips = screen.getByRole("group", { name: "admin.seekIndex.viewsLabel" }).querySelectorAll("button");
    expect(chips).toHaveLength(6);
    expect(screen.getByTestId("seek-index-view-attention")).toHaveTextContent("admin.seekIndex.viewAttention:7");
    expect(screen.getByTestId("seek-index-view-all")).toHaveTextContent("admin.seekIndex.viewAll:40");
    expect(screen.getByTestId("seek-index-view-attention").getAttribute("aria-pressed")).toBe("true");
    expect(screen.getByTestId("seek-index-view-backups").getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(screen.getByTestId("seek-index-view-backups"));
    expect(p.setView).toHaveBeenCalledWith("backups");
  });

  it("search and sort report through their setters, the scan date is local", () => {
    const p = mount({ query: "ven" });
    const search = screen.getByTestId("seek-index-search") as HTMLInputElement;
    expect(search.value).toBe("ven");
    fireEvent.change(search, { target: { value: "rocket" } });
    expect(p.setQuery).toHaveBeenCalledWith("rocket");

    const select = screen.getByLabelText("admin.seekIndex.sortLabel") as HTMLSelectElement;
    expect(select.value).toBe("priority");
    fireEvent.change(select, { target: { value: "size" } });
    expect(p.setSort).toHaveBeenCalledWith("size");

    const expected = new Date(1700000000000).toLocaleString();
    expect(screen.getByTestId("seek-index-scanned-at")).toHaveTextContent(`admin.seekIndex.scannedAt:${expected}`);
  });
});
