import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { PageHeader } from "./PageHeader";

describe("PageHeader", () => {
  it("renders title", () => {
    render(React.createElement(PageHeader, { title: "Movies" }));
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Movies");
  });

  it("renders subtitle when provided", () => {
    render(React.createElement(PageHeader, { title: "Movies", subtitle: "All films" }));
    expect(screen.getByText("All films")).toBeInTheDocument();
  });

  it("does not render subtitle when omitted", () => {
    const { container } = render(React.createElement(PageHeader, { title: "Movies" }));
    expect(container.querySelectorAll("p")).toHaveLength(0);
  });

  it("renders actions slot", () => {
    const action = React.createElement("button", null, "Add");
    render(React.createElement(PageHeader, { title: "Movies", actions: action }));
    expect(screen.getByRole("button", { name: "Add" })).toBeInTheDocument();
  });
});
