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
// The extended demo (a private instance for an outside evaluator) reports demoMode
// FALSE - nothing is locked client-side there - and only asks for a banner.
const ExtendedDemoContext = createContext(false);

export function DemoModeProvider({ children }: { children: ReactNode }) {
  const { data } = useQuery<{ demoMode?: boolean; demoExtended?: boolean; demoLocked?: string[] }>({
    queryKey: ["setup-status"],
    queryFn: () => adminSystemServiceGetSetupStatus(),
    staleTime: 60_000,
  });
  const demo = data?.demoMode === true;
  // The shared stores (locale, theme) read the flag too: on the demo they
  // keep this browser's choice instead of following the shared profile.
  useEffect(() => {
    useDemoStore.getState().setDemoMode(demo);
    useDemoStore.getState().setExtendedDemo(data?.demoExtended === true);
    useDemoStore.getState().setDemoLocked(data?.demoLocked ?? []);
  }, [demo, data?.demoExtended, data?.demoLocked]);
  return createElement(
    DemoModeContext.Provider,
    { value: demo },
    createElement(ExtendedDemoContext.Provider, { value: data?.demoExtended === true }, children),
  );
}

export function useDemoMode(): boolean {
  return useContext(DemoModeContext);
}

export function useExtendedDemo(): boolean {
  return useContext(ExtendedDemoContext);
}

// useDemoLock is the one-token way to grey a write control on the demo:
// spread the returned props LAST on the Button. Off the demo it is an empty
// object and the control is exactly what it was.
export function useDemoLock(): { demo: boolean; lock: { disabled?: true; title?: string } } {
  const demo = useDemoMode();
  const { t } = useTranslation();
  return { demo, lock: demo ? { disabled: true, title: t("common.demo.locked") } : {} };
}

// useNetworkLock greys the network settings (remote access, proxies, HTTPS) on
// both demos: on the extended demo a guest can use everything else, but a wrong
// value there closes the door on every later visitor, itself included. The
// server refuses the save either way; this only greys the control.
export function useNetworkLock(): { locked: boolean; lock: { disabled?: true; title?: string } } {
  const demo = useDemoMode();
  const extended = useExtendedDemo();
  const { t } = useTranslation();
  const locked = demo || extended;
  return { locked, lock: locked ? { disabled: true, title: t("common.demo.locked") } : {} };
}

// useDemoLockFor greys one control by what the server refuses: everything on
// the public demo, the RPC or route `key` names on the extended one.
export function useDemoLockFor(key: string): { locked: boolean; lock: { disabled?: true; title?: string } } {
  const demo = useDemoMode();
  const listed = useDemoStore((s) => s.demoLocked.includes(key));
  const { t } = useTranslation();
  const locked = demo || listed;
  return { locked, lock: locked ? { disabled: true, title: t("common.demo.locked") } : {} };
}
