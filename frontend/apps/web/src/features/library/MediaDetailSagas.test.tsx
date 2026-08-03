import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";

// Sagas were only reachable from the Collections screen: someone looking at
// Alien had no way to ask "what else of this do I have".
const listForItem = vi.fn();

vi.mock("@evey/api", () => ({
  useCollectionServiceListCollectionsForMediaItem: (...a: unknown[]) => listForItem(...(a as [])),
}));
vi.mock("@tanstack/react-router", () => ({
  // Forwards the rest of the props: the real Link does, and the test asserts
  // on data-testid.
  Link: ({ children, params, to, ...rest }: { children: React.ReactNode; params?: { collectionId?: string }; to?: string }) =>
    React.createElement("a", { href: `/collections/${params?.collectionId ?? ""}`, ...rest }, children),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import { SagaSection } from "./MediaDetailSagas";

describe("SagaSection", () => {
  it("names each saga the title belongs to and links to it", () => {
    listForItem.mockReturnValue({
      data: { collections: [{ id: "col-1", name: "Alien - Saga", itemCount: 4 }] },
    });
    render(<SagaSection mediaItemId="mi-1" />);
    expect(screen.getByTestId("media-saga-col-1")).toHaveAttribute("href", "/collections/col-1");
    expect(screen.getByText("Alien - Saga")).toBeInTheDocument();
  });

  it("renders nothing for a standalone title - the common case", () => {
    listForItem.mockReturnValue({ data: { collections: [] } });
    const { container } = render(<SagaSection mediaItemId="mi-2" />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing while the answer is unknown", () => {
    listForItem.mockReturnValue({ data: undefined });
    const { container } = render(<SagaSection mediaItemId="mi-3" />);
    expect(container).toBeEmptyDOMElement();
  });
});
