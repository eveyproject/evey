import { createFileRoute } from "@tanstack/react-router";
import { ContinueWatchingRow } from "../../features/home/ContinueWatchingRow";
import { RecentlyAddedRow } from "../../features/home/RecentlyAddedRow";
import { RecommendedRow } from "../../features/home/RecommendedRow";

export const Route = createFileRoute("/_auth/home")({
  component: HomePage,
});

function HomePage() {
  return (
    <div className="px-2 py-4">
      <ContinueWatchingRow />
      <RecentlyAddedRow />
      <RecommendedRow />
    </div>
  );
}
