// Stub for @evey/api when generated files are not available (worktrees)
// Tests should vi.mock("@evey/api") with specific return values
export const usePlaybackServiceGetNextUp = () => ({ data: undefined, isLoading: false });
export const usePlaybackServiceMarkAsWatched = () => ({ mutate: () => {} });
export const usePlaybackServiceMarkAsUnwatched = () => ({ mutate: () => {} });
export const libraryServiceGetMediaItem = () => Promise.resolve({});
