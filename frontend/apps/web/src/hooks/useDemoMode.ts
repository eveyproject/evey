import { createContext, createElement, useContext, useEffect, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { adminSystemServiceGetSetupStatus } from "@evey/api";
import { useDemoStore } from "@evey/store";

// Is this the public demo instance? The server says so on its public
// setup-status answer (EVEY_DEMO=1), the same query the root layout already
// primes, so it costs no extra request. The server refuses every locked
// operation whether or not the UI honours this flag; the flag only lets the UI
// grey the control instead of letting the click fail.
//
// A context rather than a query in every consumer: some thirty admin surfaces
// read it, and a context defaults to "not the demo" wherever nothing provides
// it - a component rendered on its own, in a test or a storybook, behaves
// exactly as before.
const DemoModeContext = createContext(false);

export function DemoModeProvider({ children }: { children: ReactNode }) {
  const { data } = useQuery<{ demoMode?: boolean }>({
    queryKey: ["setup-status"],
    queryFn: () => adminSystemServiceGetSetupStatus(),
    staleTime: 60_000,
  });
  const demo = data?.demoMode === true;
  // The shared stores (locale, theme) read the flag too: on the demo they
  // keep this browser's choice instead of following the shared profile.
  useEffect(() => {
    useDemoStore.getState().setDemoMode(demo);
  }, [demo]);
  return createElement(DemoModeContext.Provider, { value: demo }, children);
}

export function useDemoMode(): boolean {
  return useContext(DemoModeContext);
}

// useDemoLock is the one-token way to grey a write control on the demo:
// spread the returned props LAST on the Button. Off the demo it is an empty
// object and the control is exactly what it was.
export function useDemoLock(): { demo: boolean; lock: { disabled?: true; title?: string } } {
  const demo = useDemoMode();
  const { t } = useTranslation();
  return { demo, lock: demo ? { disabled: true, title: t("common.demo.locked") } : {} };
}
