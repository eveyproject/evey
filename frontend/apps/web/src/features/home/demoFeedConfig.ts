import { useCallback, useEffect, useState } from "react";

// The home layout of a demo visitor. On the demo the profile - and so its
// feed configuration - is shared by everyone signed in at once, and the
// server refuses to write it. The visitor's own layout lives in this browser
// instead: null means "no local layout yet, show the shared one".
//
// Same shape as the server's FeedConfig rows so the panel and the home page
// treat both sources alike.
export interface DemoFeedRow {
  feedType: string;
  enabled: boolean;
  displayOrder: number;
  // Never set locally; present so a local row reads like a server one.
  maxItems?: number;
}

const KEY = "evey-demo-feed";
const EVENT = "evey-demo-feed-change";

export function readDemoFeedConfig(): DemoFeedRow[] | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) ? (parsed as DemoFeedRow[]) : null;
  } catch {
    return null;
  }
}

export function writeDemoFeedConfig(rows: DemoFeedRow[] | null): void {
  try {
    if (rows) localStorage.setItem(KEY, JSON.stringify(rows));
    else localStorage.removeItem(KEY);
  } catch {
    // Private window or blocked storage: the layout lasts the session only.
  }
  window.dispatchEvent(new Event(EVENT));
}

// useDemoFeedConfig: the local rows and a writer. The panel writes, the home
// page reads, both mounted at once - the event keeps them in step.
export function useDemoFeedConfig(): [DemoFeedRow[] | null, (rows: DemoFeedRow[] | null) => void] {
  const [rows, setRows] = useState<DemoFeedRow[] | null>(() => readDemoFeedConfig());
  useEffect(() => {
    const onChange = () => setRows(readDemoFeedConfig());
    window.addEventListener(EVENT, onChange);
    return () => window.removeEventListener(EVENT, onChange);
  }, []);
  const write = useCallback((next: DemoFeedRow[] | null) => writeDemoFeedConfig(next), []);
  return [rows, write];
}
