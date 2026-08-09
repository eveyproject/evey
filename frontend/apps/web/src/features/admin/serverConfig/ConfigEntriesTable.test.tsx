import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { ConfigRow } from "./ConfigEntriesTable";

const deleteMutate = vi.fn();
const setMutate = vi.fn();
vi.mock("./hooks", () => ({
  useSetConfigEntry: () => ({ mutate: setMutate, isPending: false }),
  useDeleteConfigEntry: () => ({ mutate: deleteMutate, isPending: false }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

beforeEach(() => {
  vi.clearAllMocks();
});

function renderRow() {
  render(
    <table>
      <tbody>
        <ConfigRow entry={{ key: "some.flag", value: "on" }} />
      </tbody>
    </table>,
  );
}

// Confirm-to-act on the delete button (window.confirm replaced): the first
// click only ARMS, the second click deletes.
describe("ConfigRow delete confirm-to-act", () => {
  it("a single click arms the button and deletes nothing", () => {
    renderRow();
    fireEvent.click(screen.getByRole("button", { name: "common.delete" }));
    expect(deleteMutate).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "common.confirm" })).toBeInTheDocument();
  });

  it("the second click deletes the entry by key", () => {
    renderRow();
    fireEvent.click(screen.getByRole("button", { name: "common.delete" }));
    fireEvent.click(screen.getByRole("button", { name: "common.confirm" }));
    expect(deleteMutate).toHaveBeenCalledWith("some.flag");
  });

  it("the armed state times out back to Delete", () => {
    vi.useFakeTimers();
    try {
      renderRow();
      fireEvent.click(screen.getByRole("button", { name: "common.delete" }));
      act(() => {
        vi.advanceTimersByTime(4100);
      });
      expect(screen.getByRole("button", { name: "common.delete" })).toBeInTheDocument();
      expect(deleteMutate).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });
});
