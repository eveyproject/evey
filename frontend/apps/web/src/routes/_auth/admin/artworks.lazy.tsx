import { createLazyFileRoute } from "@tanstack/react-router";
import { ArtworksAdmin } from "@/features/admin/ArtworksAdmin";

export const Route = createLazyFileRoute("/_auth/admin/artworks")({
  component: ArtworksAdmin,
});
