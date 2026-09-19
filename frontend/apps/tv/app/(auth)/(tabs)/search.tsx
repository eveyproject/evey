import { SearchPanel } from "../../../src/components/SearchPanel";

// The search as a full page. The everyday way in is now the header's dropdown
// (SearchOverlay), which mounts the very same panel; this route stays for what
// still links to it and for anyone arriving by deep link.
//
// selectSearchResultsView is re-exported because it is this screen's contract
// as much as the panel's: exactly one results view, so exactly one initial
// focus claimant.
export { selectSearchResultsView } from "../../../src/components/SearchPanel";
export type { SearchResultsView } from "../../../src/components/SearchPanel";

export default function SearchScreen() {
  // The field takes the remote on arrival: this route exists to be typed into,
  // and a screen that claims nothing leaves Android with no focused view.
  return <SearchPanel hasTVPreferredFocus />;
}
