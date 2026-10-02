import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { createMemoryHistory, createRootRoute, createRoute, createRouter, RouterProvider } from "@tanstack/react-router";
import { NotFoundPage } from "./NotFoundPage";

// The router's own fallback was a bare "Not Found" without the app around it;
// an unknown address now gets the app's empty state and a way home.
describe("NotFoundPage", () => {
  it("answers an unknown address and leads home", async () => {
    const root = createRootRoute();
    const home = createRoute({ getParentRoute: () => root, path: "/", component: () => <p>home page</p> });
    const router = createRouter({
      routeTree: root.addChildren([home]),
      history: createMemoryHistory({ initialEntries: ["/nowhere"] }),
      defaultNotFoundComponent: NotFoundPage,
    });
    render(<RouterProvider router={router} />);

    expect(await screen.findByText("Page not found")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Go Home" }));
    expect(await screen.findByText("home page")).toBeInTheDocument();
  });
});
