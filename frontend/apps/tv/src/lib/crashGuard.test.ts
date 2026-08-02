import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// "Aucune erreur ne doit faire planter l'app" is the requirement; this file is
// the proof. The interesting cases are the two that differ: in development the
// original handler MUST still fire (the red box is the diagnosis), in a shipped
// build it must NOT (calling it is what tears the app down).

const logged: string[] = [];
vi.mock("./log", () => ({
  logger: () => ({
    error: (...a: unknown[]) => logged.push(a.join(" ")),
    info: () => {},
    warn: () => {},
  }),
  recentLogs: () => ["ligne 1", "ligne 2"],
}));

type Handler = (error: unknown, isFatal?: boolean) => void;

const g = globalThis as Record<string, unknown>;

/** Fresh module per test: installCrashGuard is deliberately install-once. */
async function loadGuard() {
  vi.resetModules();
  return import("./crashGuard");
}

function stubErrorUtils(previous?: Handler) {
  let current = previous;
  g.ErrorUtils = {
    getGlobalHandler: () => current,
    setGlobalHandler: (h: Handler) => {
      current = h;
    },
  };
  return () => current;
}

describe("crashGuard", () => {
  beforeEach(() => {
    logged.length = 0;
    delete g.onunhandledrejection;
  });
  afterEach(() => {
    delete g.ErrorUtils;
    delete g.__DEV__;
    delete g.onunhandledrejection;
  });

  it("installe un gestionnaire global", async () => {
    const handler = stubErrorUtils();
    const { installCrashGuard } = await loadGuard();
    installCrashGuard();
    expect(typeof handler()).toBe("function");
  });

  // The whole point: a shipped build must survive the error, so the previous
  // handler (React Native's, which ends the process) is deliberately not called.
  it("avale l'erreur en build livre au lieu de laisser mourir l'app", async () => {
    const previous = vi.fn();
    const handler = stubErrorUtils(previous);
    g.__DEV__ = false;
    const { installCrashGuard } = await loadGuard();
    installCrashGuard();

    expect(() => handler()!(new Error("boum"), true)).not.toThrow();
    expect(previous).not.toHaveBeenCalled();
    expect(logged.join("\n")).toContain("boum");
    expect(logged.join("\n")).toContain("fatal");
  });

  it("laisse la red box faire son travail en developpement", async () => {
    const previous = vi.fn();
    const handler = stubErrorUtils(previous);
    g.__DEV__ = true;
    const { installCrashGuard } = await loadGuard();
    installCrashGuard();

    const err = new Error("boum");
    handler()!(err, false);
    expect(previous).toHaveBeenCalledWith(err, false);
  });

  it("journalise les rejets de promesse que personne n'a traites, sans voler le handler existant", async () => {
    stubErrorUtils();
    const previousRejection = vi.fn();
    g.onunhandledrejection = previousRejection;
    g.__DEV__ = false;
    const { installCrashGuard } = await loadGuard();
    installCrashGuard();

    const event = { reason: new Error("promesse perdue") };
    (g.onunhandledrejection as (e: unknown) => void)(event);
    expect(logged.join("\n")).toContain("promesse perdue");
    expect(previousRejection).toHaveBeenCalledWith(event);
  });

  it("ne s'installe qu'une fois", async () => {
    const handler = stubErrorUtils();
    const { installCrashGuard } = await loadGuard();
    installCrashGuard();
    const first = handler();
    installCrashGuard();
    expect(handler()).toBe(first);
  });

  // A television has no debugger attached: whatever the crash screen shows is
  // the entire diagnosis, so the tail of the log has to be reachable.
  it("expose la fin du journal pour un ecran de secours", async () => {
    stubErrorUtils();
    const { crashContext } = await loadGuard();
    expect(crashContext()).toContain("ligne 2");
  });

  // The guard runs before anything else, on a platform where ErrorUtils may
  // simply not be there. Blowing up here would be the crash it exists to stop.
  it("survit a l'absence d'ErrorUtils", async () => {
    delete g.ErrorUtils;
    const { installCrashGuard } = await loadGuard();
    expect(() => installCrashGuard()).not.toThrow();
  });
});
