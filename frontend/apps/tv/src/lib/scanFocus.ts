// Where the remote's focus goes during a LAN scan on the server-connect
// screen. Extracted so the D-pad contract can be tested without mounting the
// screen (it drags the whole server store, the fetcher and i18n in).
//
// The rules, from using it on a television (2026-09-19): pressing Scan must
// land the focus ON the first result that arrives - the scan button otherwise
// keeps it and the rows appear below the fold with nothing saying they are
// there. Picking a result FILLS the URL field (it does not connect), so the
// focus moves to that field: the viewer sees what changed and that Connect
// is the next press.

export interface ScanFocusState {
  // A scan is armed: the next first-row render takes the focus.
  focusFirstResult: boolean;
}

export const scanFocusIdle: ScanFocusState = { focusFirstResult: false };

/** A scan starts: arm the handoff (results are cleared by the caller). */
export function onScanStarted(): ScanFocusState {
  return { focusFirstResult: true };
}

/** Does THIS row claim the focus as it renders? Only the first, only once. */
export function rowClaimsFocus(state: ScanFocusState, index: number): boolean {
  return state.focusFirstResult && index === 0;
}

/** A result was picked: the field takes over, the handoff is spent. */
export function onResultPicked(): ScanFocusState {
  return scanFocusIdle;
}
