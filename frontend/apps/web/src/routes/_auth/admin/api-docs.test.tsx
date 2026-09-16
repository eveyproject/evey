import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const fetchRaw = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetchRaw: (...a: unknown[]) => fetchRaw(...a) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));
// SwaggerUI is replaced by a probe that exposes the sorter it was handed.
const swagger = vi.hoisted(() => ({ props: null as any }));
vi.mock("swagger-ui-react", () => ({
  default: (p: any) => {
    swagger.props = p;
    return React.createElement("div", { "data-testid": "swagger" }, JSON.stringify(p.spec));
  },
}));
vi.mock("swagger-ui-react/swagger-ui.css", () => ({}));

import { Route } from "./api-docs.lazy";
const Page = (Route as unknown as { component: React.ComponentType }).component;

const op = (method: string, path: string) => ({ get: (k: string) => (k === "method" ? method : path) });

beforeEach(() => {
  fetchRaw.mockReset();
  swagger.props = null;
});

// The spec sits behind auth, so it is fetched with the bearer and handed to
// SwaggerUI as an object; a refused fetch names its reason; operations sort
// by REST verb then by path so the reference reads the same every time.
describe("api-docs page", () => {
  it("loads the spec through the authenticated fetch and orders operations by verb then path", async () => {
    fetchRaw.mockResolvedValue({ json: async () => ({ openapi: "3.0.0" }) });
    render(<Page />);
    await waitFor(() => expect(screen.getByTestId("swagger")).toHaveTextContent('{"openapi":"3.0.0"}'));
    expect(fetchRaw).toHaveBeenCalledWith({ url: "/api/v1/openapi.json", method: "GET" });
    expect(swagger.props.docExpansion).toBe("list");
    const sort = swagger.props.operationsSorter as (a: unknown, b: unknown) => number;
    expect(sort(op("post", "/a"), op("get", "/z"))).toBeGreaterThan(0);
    expect(sort(op("get", "/a"), op("get", "/b"))).toBeLessThan(0);
    expect(sort(op("delete", "/a"), op("options", "/a"))).toBeLessThan(0);
    expect(screen.queryByText("auth.apiDocs.loadError")).toBeNull();
  });

  it("names the failure when the spec cannot be fetched", async () => {
    fetchRaw.mockRejectedValue(new Error("HTTP 403"));
    render(<Page />);
    await waitFor(() => expect(screen.getByText("auth.apiDocs.loadError")).toBeInTheDocument());
    expect(screen.getByText("(HTTP 403)")).toBeInTheDocument();
    expect(screen.queryByTestId("swagger")).toBeNull();
  });
});
