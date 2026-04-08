import { describe, it, expect, beforeEach } from "vitest";
import { useToastStore, toast } from "@evey/store";

function reset() {
  useToastStore.setState({ toasts: [] });
}

describe("useToastStore", () => {
  beforeEach(reset);

  it("starts with empty toasts", () => {
    expect(useToastStore.getState().toasts).toHaveLength(0);
  });

  describe("addToast", () => {
    it("appends toast with generated id", () => {
      const id = useToastStore.getState().addToast({ type: "info", message: "hello" });
      const toasts = useToastStore.getState().toasts;
      expect(toasts).toHaveLength(1);
      expect(toasts[0]).toMatchObject({ id, type: "info", message: "hello" });
    });

    it("returns unique ids", () => {
      const id1 = useToastStore.getState().addToast({ type: "info", message: "a" });
      const id2 = useToastStore.getState().addToast({ type: "info", message: "b" });
      expect(id1).not.toBe(id2);
    });

    it("preserves optional duration", () => {
      const id = useToastStore.getState().addToast({ type: "error", message: "oops", duration: 3000 });
      const t = useToastStore.getState().toasts.find((x) => x.id === id);
      expect(t?.duration).toBe(3000);
    });

    it("caps at 5 toasts, dropping oldest", () => {
      for (let i = 0; i < 7; i++) {
        useToastStore.getState().addToast({ type: "info", message: `msg-${i}` });
      }
      const toasts = useToastStore.getState().toasts;
      expect(toasts).toHaveLength(5);
      expect(toasts[0].message).toBe("msg-2");
      expect(toasts[4].message).toBe("msg-6");
    });

    it("allows exactly 5 without dropping", () => {
      for (let i = 0; i < 5; i++) {
        useToastStore.getState().addToast({ type: "success", message: `m-${i}` });
      }
      expect(useToastStore.getState().toasts).toHaveLength(5);
    });
  });

  describe("removeToast", () => {
    it("removes by id", () => {
      const id = useToastStore.getState().addToast({ type: "success", message: "ok" });
      useToastStore.getState().removeToast(id);
      expect(useToastStore.getState().toasts).toHaveLength(0);
    });

    it("is a no-op for unknown id", () => {
      useToastStore.getState().addToast({ type: "info", message: "keep" });
      useToastStore.getState().removeToast("non-existent");
      expect(useToastStore.getState().toasts).toHaveLength(1);
    });

    it("removes only the matching toast", () => {
      const id1 = useToastStore.getState().addToast({ type: "info", message: "first" });
      useToastStore.getState().addToast({ type: "info", message: "second" });
      useToastStore.getState().removeToast(id1);
      const toasts = useToastStore.getState().toasts;
      expect(toasts).toHaveLength(1);
      expect(toasts[0].message).toBe("second");
    });
  });
});

describe("toast helpers", () => {
  beforeEach(reset);

  it("toast.success adds success type", () => {
    toast.success("done");
    const t = useToastStore.getState().toasts[0];
    expect(t.type).toBe("success");
    expect(t.message).toBe("done");
  });

  it("toast.error adds error type with 6000ms duration", () => {
    toast.error("fail");
    const t = useToastStore.getState().toasts[0];
    expect(t.type).toBe("error");
    expect(t.message).toBe("fail");
    expect(t.duration).toBe(6000);
  });

  it("toast.info adds info type", () => {
    toast.info("fyi");
    const t = useToastStore.getState().toasts[0];
    expect(t.type).toBe("info");
    expect(t.message).toBe("fyi");
  });

  it("toast.warning adds warning type", () => {
    toast.warning("careful");
    const t = useToastStore.getState().toasts[0];
    expect(t.type).toBe("warning");
    expect(t.message).toBe("careful");
  });

  it("each helper returns a toast id string", () => {
    const id = toast.success("test");
    expect(typeof id).toBe("string");
    expect(id).toMatch(/^toast-/);
  });
});
