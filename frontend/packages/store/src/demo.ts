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
}

export const useDemoStore = create<DemoState>()((set) => ({
  demoMode: false,
  setDemoMode: (demoMode) => set({ demoMode }),
}));

export function isDemoMode(): boolean {
  return useDemoStore.getState().demoMode;
}
