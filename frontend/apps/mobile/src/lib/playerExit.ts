// playerCloseTarget / playerExit are shared with the web and the television
// (packages/client/src/lib/playerExit.ts); re-exported for this app's callers.
export { playerCloseTarget, playerExit, type PlayerExit } from "@evey/client";

// Hardware back closes what is OPEN before it closes the PLAYER. The settings
// panel is a plain positioned View - unlike the sheets, which are native
// Modals whose onRequestClose intercepts the back press before this handler
// ever runs - so without this decision a back press with the panel up tears
// down playback instead of dismissing the panel.
export type BackPressAction = "close-settings" | "exit";

export function backPressAction(settingsOpen: boolean): BackPressAction {
  return settingsOpen ? "close-settings" : "exit";
}
