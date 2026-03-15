import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import type { Collection } from "./api";

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to,
    params,
    ...rest
  }: {
    children: React.ReactNode;
    to: string;
    params?: Record<string, string>;
    [k: string]: unknown;
  }) => {
    const href = params?.collectionId ? to.replace("$collectionId", params.collectionId) : to;
    return (
      <a href={href} data-testid={rest["data-testid"] as string} className={rest["className"] as string}>
        {children}
      </a>
    );
  },
}));

const baseCollection: Collection = {
  id: "col-1",
  name: "Favorites",
  createdAt: "2024-01-01T00:00:00Z",
  updatedAt: "2024-01-01T00:00:00Z",
};

describe("CollectionCard", () => {
  it("renders collection name and link to collection page", async () => {
    const { CollectionCard } = await import("./CollectionCard");
    render(<CollectionCard collection={baseCollection} />);

    expect(screen.getByText("Favorites")).toBeInTheDocument();
    expect(screen.getByTestId("collection-card")).toHaveAttribute("href", "/collections/col-1");
  });

  it("shows initial letter when no posterUrl", async () => {
    const { CollectionCard } = await import("./CollectionCard");
    render(<CollectionCard collection={{ ...baseCollection, posterUrl: undefined }} />);

    expect(screen.getByText("F")).toBeInTheDocument();
  });

  it("renders poster image when posterUrl is provided", async () => {
    const { CollectionCard } = await import("./CollectionCard");
    render(<CollectionCard collection={{ ...baseCollection, posterUrl: "https://example.com/poster.jpg" }} />);

    const img = screen.getByRole("img", { name: "Favorites" });
    expect(img).toHaveAttribute("src", "https://example.com/poster.jpg");
  });

  it("falls back to initial letter when image fails to load", async () => {
    const { CollectionCard } = await import("./CollectionCard");
    render(<CollectionCard collection={{ ...baseCollection, posterUrl: "https://example.com/bad.jpg" }} />);

    const img = screen.getByRole("img", { name: "Favorites" });
    fireEvent.error(img);

    expect(screen.getByText("F")).toBeInTheDocument();
  });

  it("shows '?' as initial for empty collection name", async () => {
    const { CollectionCard } = await import("./CollectionCard");
    render(<CollectionCard collection={{ ...baseCollection, name: "" }} />);

    expect(screen.getByText("?")).toBeInTheDocument();
  });

  it("image becomes visible after load event", async () => {
    const { CollectionCard } = await import("./CollectionCard");
    render(<CollectionCard collection={{ ...baseCollection, posterUrl: "https://example.com/poster.jpg" }} />);

    const img = screen.getByRole("img", { name: "Favorites" });
    expect(img).toHaveStyle({ opacity: "0" });

    fireEvent.load(img);
    expect(img).toHaveStyle({ opacity: "1" });
  });
});
