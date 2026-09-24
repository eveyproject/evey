import { useCallback } from "react";
import { FocusRoot } from "../tv/focus";
import { SearchPanel } from "../components/SearchPanel";
import { replace } from "../app/router";

// The search tab: the header's panel as a page. The query rides the url so
// back returns to the same results.
export function SearchScreen({ initialQuery }: { initialQuery?: string }) {
  const keepQuery = useCallback((q: string) => replace({ name: "search", query: q.trim() || undefined }), []);
  return (
    <FocusRoot autoFocus className="tab-screen">
      <SearchPanel initialQuery={initialQuery} onQueryChange={keepQuery} />
    </FocusRoot>
  );
}
