import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { WatcherFilter, encodeWatcher, decodeWatcher } from "./WatcherFilter";

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useUserServiceListUsers: () => ({
    data: { users: [{ id: "usr-1", username: "alice" }, { id: "usr-2", username: "bob" }] },
  }),
  useUserServiceListAllProfiles: () => ({
    data: {
      profiles: [
        { id: "prf-1", userId: "usr-1", name: "Papa" },
        { id: "prf-2", userId: "usr-1", name: "Les enfants" },
        { id: "prf-3", userId: "usr-2", name: "Bob" },
      ],
    },
  }),
}));

describe("WatcherFilter", () => {
  beforeEach(() => vi.clearAllMocks());

  // The encoding is what keeps a single <select> able to mean two different
  // API filters; a round-trip that loses which one is which would silently
  // widen a profile filter to the whole account.
  it("round-trips both kinds of watcher, and nothing for everyone", () => {
    expect(decodeWatcher(encodeWatcher({ profileId: "prf-1" }))).toEqual({ profileId: "prf-1" });
    expect(decodeWatcher(encodeWatcher({ userId: "usr-1" }))).toEqual({ userId: "usr-1" });
    expect(encodeWatcher({})).toBe("");
    expect(decodeWatcher("")).toEqual({});
  });

  it("groups each account's profiles under it", () => {
    render(<WatcherFilter value={{}} onChange={vi.fn()} />);
    expect(screen.getByRole("group", { name: "alice" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Les enfants" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Bob" })).toBeInTheDocument();
  });

  it("reports the picked profile as a profile filter", () => {
    const onChange = vi.fn();
    render(<WatcherFilter value={{}} onChange={onChange} />);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "profile:prf-2" } });
    expect(onChange).toHaveBeenCalledWith({ profileId: "prf-2" });
  });

  it("reports a whole account as a user filter", () => {
    const onChange = vi.fn();
    render(<WatcherFilter value={{}} onChange={onChange} />);
    fireEvent.change(screen.getByRole("combobox"), { target: { value: "user:usr-2" } });
    expect(onChange).toHaveBeenCalledWith({ userId: "usr-2" });
  });
});
