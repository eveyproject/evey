import { createLazyFileRoute } from "@tanstack/react-router";
import { ArtworksAdmin } from "@/features/admin/ArtworksAdmin";
import { DemoReadOnly } from "@/components/layout/DemoReadOnly";

export const Route = createLazyFileRoute("/_auth/admin/artworks")({
  // Every control here is an operator write the demo refuses (DemoReadOnly).
  component: () => (
    <DemoReadOnly>
      <ArtworksAdmin />
    </DemoReadOnly>
  ),
});
