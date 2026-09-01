import { createFileRoute } from "@tanstack/react-router";

// The users panel deep-links here to answer "what did THIS profile watch?", so
// the filter has to survive in the URL rather than live in component state.
type ActivitySearch = {
  tab?: "play" | "audit" | "stats";
  userId?: string;
  profileId?: string;
};

export const Route = createFileRoute("/_auth/admin/activity")({
  validateSearch: (search: Record<string, unknown>): ActivitySearch => {
    const tab = typeof search.tab === "string" ? search.tab : undefined;
    return {
      tab: tab === "play" || tab === "audit" || tab === "stats" ? tab : undefined,
      userId: typeof search.userId === "string" ? search.userId : undefined,
      profileId: typeof search.profileId === "string" ? search.profileId : undefined,
    };
  },
});
