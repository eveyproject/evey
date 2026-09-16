import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const params = vi.hoisted(() => ({ pluginId: "p1" }));
vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => ({ ...opts, useParams: () => params }),
  Link: ({ children, to }: any) => React.createElement("a", { href: to }, children),
}));
const entries = vi.hoisted(() => ({ list: [] as any[], slot: "" }));
vi.mock("@/features/plugins/usePluginSlot", () => ({
  usePluginSlotEntries: (slot: string) => {
    entries.slot = slot;
    return entries.list;
  },
}));
vi.mock("@/components/ui/PageHeader", () => ({ PageHeader: ({ title }: any) => React.createElement("h1", null, title) }));
vi.mock("@/features/plugins/PluginIframe", () => ({
  PluginIframe: (p: any) => React.createElement("iframe", { "data-testid": "frame", "data-plugin": p.pluginId, src: p.src, "data-external": String(p.externalFetch) }),
}));

import { Route } from "./extensions.$pluginId.lazy";
const Page = (Route as unknown as { component: React.ComponentType }).component;

beforeEach(() => {
  entries.list = [];
  params.pluginId = "p1";
});

// The page hosts exactly the plugin named in the URL, from the admin.nav
// slot, in its iframe with its own external-fetch policy; a plugin that is
// not installed, disabled, or without an admin module gets a plain notice
// and no frame, with the way back to the plugins list either way.
describe("plugin extension page", () => {
  it("frames the plugin named by the URL", () => {
    entries.list = [
      { pluginId: "other", name: "Other", entryPath: "/plugins/other/admin.html", externalFetch: false },
      { pluginId: "p1", name: "Stats", entryPath: "/plugins/p1/admin.html", externalFetch: true },
    ];
    render(<Page />);
    expect(entries.slot).toBe("admin.nav");
    expect(screen.getByRole("heading")).toHaveTextContent("Stats");
    const frame = screen.getByTestId("frame");
    expect(frame.getAttribute("data-plugin")).toBe("p1");
    expect(frame.getAttribute("src")).toBe("/plugins/p1/admin.html");
    expect(frame.getAttribute("data-external")).toBe("true");
    expect(screen.getByRole("link").getAttribute("href")).toBe("/admin/plugins");
  });

  it("explains an unknown plugin instead of framing anything", () => {
    params.pluginId = "ghost";
    entries.list = [{ pluginId: "p1", name: "Stats", entryPath: "/x", externalFetch: false }];
    render(<Page />);
    expect(screen.queryByTestId("frame")).toBeNull();
    expect(screen.getByText(/not installed, not enabled, or has no admin page/)).toBeInTheDocument();
    expect(screen.getByRole("link").getAttribute("href")).toBe("/admin/plugins");
  });
});
