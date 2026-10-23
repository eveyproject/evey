import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

const demo = vi.hoisted(() => ({ locked: false }));
vi.mock("@tanstack/react-router", () => ({ createLazyFileRoute: () => (o: { component: unknown }) => ({ options: o }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@/hooks/useDemoMode", () => ({
  useDemoLockFor: (key: string) => {
    expect(key).toBe("GET /api/v1/admin/logs");
    return { locked: demo.locked, lock: {} };
  },
}));
vi.mock("@/features/admin/LogsExplorer", () => ({ LogsExplorer: () => <div data-testid="explorer" /> }));
vi.mock("@/components/ui/PageHeader", () => ({ PageHeader: ({ title }: { title: string }) => <h1>{title}</h1> }));
vi.mock("@/components/ui/EmptyState", () => ({ EmptyState: ({ title }: { title: string }) => <div data-testid="locked">{title}</div> }));

import { Route } from "./logs.lazy";

const Page = (Route as unknown as { options: { component: React.ComponentType } }).options.component;

beforeEach(() => {
  demo.locked = false;
});

describe("admin logs page", () => {
  it("mounts the explorer when the logs are readable", () => {
    render(<Page />);
    expect(screen.getByTestId("explorer")).toBeInTheDocument();
    expect(screen.queryByTestId("locked")).toBeNull();
  });

  it("shows the locked state instead of firing requests the server refuses", () => {
    demo.locked = true;
    render(<Page />);
    expect(screen.getByTestId("locked")).toHaveTextContent("errors.demo.locked");
    expect(screen.queryByTestId("explorer")).toBeNull();
  });
});
