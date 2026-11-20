import { createLazyFileRoute } from "@tanstack/react-router";
import { WatchHistory } from "@/features/user/WatchHistory";

export const Route = createLazyFileRoute("/_auth/profile/history")({
  component: HistoryPage,
});

function HistoryPage() {
  return (
    <div className="px-6 py-8 max-w-4xl">
      <WatchHistory showClearButton />
    </div>
  );
}
