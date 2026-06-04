import { describe, it, expect, beforeEach } from "vitest";
import { useToastStore, toast } from "./toast";

beforeEach(() => {
  useToastStore.setState({ toasts: [] });
});

describe("useToastStore", () => {
  it("addToast appends a toast with a generated id and returns it", () => {
    const id = useToastStore.getState().addToast({ type: "info", message: "hi" });
    const toasts = useToastStore.getState().toasts;
    expect(toasts).toHaveLength(1);
    expect(toasts[0]).toMatchObject({ id, type: "info", message: "hi" });
    expect(id).toMatch(/^toast-\d+$/);
  });

  it("generates unique ids per toast", () => {
    const a = useToastStore.getState().addToast({ type: "info", message: "a" });
    const b = useToastStore.getState().addToast({ type: "info", message: "b" });
    expect(a).not.toBe(b);
  });

  it("caps at 5 visible toasts, dropping the OLDEST (not the newest)", () => {
    const add = useToastStore.getState().addToast;
    const ids = Array.from({ length: 6 }, (_, i) => add({ type: "info", message: `m${i}` }));
    const toasts = useToastStore.getState().toasts;
    expect(toasts).toHaveLength(5);
    // the first (oldest) was dropped; the last 5 survive in insertion order
    expect(toasts.map((t) => t.id)).toEqual(ids.slice(1));
    expect(toasts.map((t) => t.message)).toEqual(["m1", "m2", "m3", "m4", "m5"]);
  });

  it("removeToast removes only the matching id", () => {
    const add = useToastStore.getState().addToast;
    const id1 = add({ type: "info", message: "1" });
    const id2 = add({ type: "info", message: "2" });
    useToastStore.getState().removeToast(id1);
    const toasts = useToastStore.getState().toasts;
    expect(toasts).toHaveLength(1);
    expect(toasts[0].id).toBe(id2);
  });

  it("removeToast on an unknown id is a no-op", () => {
    useToastStore.getState().addToast({ type: "info", message: "1" });
    useToastStore.getState().removeToast("does-not-exist");
    expect(useToastStore.getState().toasts).toHaveLength(1);
  });
});

describe("toast helpers", () => {
  it("error sets type=error with the 6s duration", () => {
    toast.error("boom");
    expect(useToastStore.getState().toasts[0]).toMatchObject({
      type: "error",
      message: "boom",
      duration: 6000,
    });
  });

  it("success/info/warning set their type with no duration", () => {
    toast.success("ok");
    toast.info("fyi");
    toast.warning("careful");
    const toasts = useToastStore.getState().toasts;
    expect(toasts.map((t) => t.type)).toEqual(["success", "info", "warning"]);
    expect(toasts.every((t) => t.duration === undefined)).toBe(true);
  });
});
