import { create } from "zustand";

// Is this client talking to the public demo instance? The server says so on
// its public setup-status answer (EVEY_DEMO=1); each app sets this once it has
// read it. Not persisted: it describes the server, and the answer is fetched
// again on every load.
//
// On the demo the account and its profiles are shared by every visitor at
// once, so the stores that normally follow the PROFILE (locale, theme) keep
// the visitor's own choice in this browser instead and ignore what the shared
// profile says - the server refuses to write those preferences there anyway.
interface DemoState {
  demoMode: boolean;
  setDemoMode: (on: boolean) => void;
  // The extended demo reports demoMode false (nothing is locked client-side);
  // this only tells the few network screens to stay hands-off.
  extendedDemo: boolean;
  setExtendedDemo: (on: boolean) => void;
  // What the extended demo still refuses (RPC names, route patterns), as the
  // server lists it.
  demoLocked: string[];
  setDemoLocked: (keys: string[]) => void;
}

export const useDemoStore = create<DemoState>()((set) => ({
  demoMode: false,
  setDemoMode: (demoMode) => set({ demoMode }),
  extendedDemo: false,
  setExtendedDemo: (extendedDemo) => set({ extendedDemo }),
  demoLocked: [],
  setDemoLocked: (demoLocked) => set({ demoLocked }),
}));

/** Refused here: anything on the public demo, the listed keys on the extended one. */
export function demoLocks(s: Pick<DemoState, "demoMode" | "demoLocked">, key: string): boolean {
  return s.demoMode || s.demoLocked.includes(key);
}

// The public demo's one account. Its credentials are published (the login
// screens show them), so the login forms fill them in on the demo.
export const DEMO_USERNAME = "demo";
export const DEMO_PASSWORD = "demo";

/**
 * Fills an untouched login form with the demo account once the server says it
 * is the demo. A field the viewer already typed in (or a saved login) is kept.
 */
export function prefillDemoLogin(
  setUsername: (update: (v: string) => string) => void,
  setPassword: (update: (v: string) => string) => void,
): void {
  setUsername((v) => v || DEMO_USERNAME);
  setPassword((v) => v || DEMO_PASSWORD);
}

export function isDemoMode(): boolean {
  return useDemoStore.getState().demoMode;
}
