import { describe, it, expect, beforeEach } from "vitest";
import { useToastStore, toast } from "@evey/store";

function resetStore() {
  useToastStore.setState({ toasts: [] });
}

describe("useToastStore", () => {
  beforeEach(resetStore);

  it("starts with empty toasts", () => {
    expect(useToastStore.getState().toasts).toHaveLength(0);
  });

  it("addToast appends a toast with generated id", () => {
    const id = useToastStore.getState().addToast({ type: "info", message: "hello" });
    const toasts = useToastStore.getState().toasts;
    expect(toasts).toHaveLength(1);
    expect(toasts[0]).toMatchObject({ id, type: "info", message: "hello" });
  });

  it("removeToast removes by id", () => {
    const id = useToastStore.getState().addToast({ type: "success", message: "ok" });
    useToastStore.getState().removeToast(id);
    expect(useToastStore.getState().toasts).toHaveLength(0);
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
});

describe("toast helpers", () => {
  beforeEach(resetStore);

  it("toast.success adds a success toast", () => {
    toast.success("done");
    const t = useToastStore.getState().toasts[0];
    expect(t.type).toBe("success");
    expect(t.message).toBe("done");
  });

  it("toast.error adds an error toast with duration", () => {
    toast.error("fail");
    const t = useToastStore.getState().toasts[0];
    expect(t.type).toBe("error");
    expect(t.duration).toBe(6000);
  });

  it("toast.info adds an info toast", () => {
    toast.info("fyi");
    expect(useToastStore.getState().toasts[0].type).toBe("info");
  });

  it("toast.warning adds a warning toast", () => {
    toast.warning("careful");
    expect(useToastStore.getState().toasts[0].type).toBe("warning");
  });
});
