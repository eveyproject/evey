// Query options for the watch progress behind a "Resume mm:ss" button. That
// progress moves while the page sits in cache - the player chains into the next
// episodes, another device plays on - so it is read every time the page is
// shown, never served stale: a TV once offered "Resume 7:06" on an episode
// finished the night before, and the press started it over at 0 (27-09).
export const RESUME_PROGRESS_QUERY = { staleTime: 0, refetchOnWindowFocus: true } as const;
