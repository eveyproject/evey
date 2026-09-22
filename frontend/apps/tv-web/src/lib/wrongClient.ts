/**
 * Whether this page is the wrong application for the machine showing it.
 *
 * `/tv` is the television client: ten-foot type, a remote-driven focus ring,
 * and a session that lasts months. Opened from a desktop browser it is
 * simply the wrong one, and the web application at `/` is right there.
 *
 * Deliberately a user-agent test and nothing cleverer. A television browser
 * says what it is, and the cost of being wrong is one redirect a viewer can
 * undo: there is no capability to probe that separates a television from a
 * large monitor, and guessing from the screen size would send a desktop at
 * 4K to the television client and a television in a window to the web one.
 */

// What the sets actually send. LG says Web0S, Samsung says Tizen, the Android
// televisions and the dongles say Android TV / CrKey / AFT, and the rest are
// the brand strings of sets that still ship their own browser.
const TELEVISION = /Web0S|webOS|SMART-TV|SmartTV|Tizen|VIDAA|HbbTV|NetCast|BRAVIA|AFT[A-Za-z]|CrKey|Android ?TV|GoogleTV|Roku|AppleTV|Chromecast|DTV|Hisense|SmartTV/i;

// A development host: nobody is redirected away from the client they are
// working on.
const DEV_HOST = /^(localhost|127\.|\[?::1\]?|0\.0\.0\.0|.*\.local|192\.168\.|10\.|172\.(1[6-9]|2\d|3[01])\.)/i;

export interface WrongClientInput {
  /** file: for the packaged application, http(s): when served under /tv. */
  protocol: string;
  hostname: string;
  userAgent: string;
  /** The query of the current address: tv=1 keeps the viewer here. */
  search: string;
  /** A previous "keep me here" answer, remembered by the browser. */
  forced: boolean;
}

/** True when the viewer should be sent to the web application instead. */
export function shouldLeaveTvClient(input: WrongClientInput): boolean {
  // The packaged application (webOS .ipk, a local file) is never redirected:
  // there is no web application next to it to redirect TO.
  if (input.protocol === "file:") return false;
  if (input.forced || /(^|[?&])tv=1(&|$)/.test(input.search)) return false;
  if (DEV_HOST.test(input.hostname)) return false;
  return !TELEVISION.test(input.userAgent);
}

/** Whether this visit asked, in its address, to stay on the television client. */
export function asksToStay(search: string): boolean {
  return /(^|[?&])tv=1(&|$)/.test(search);
}

/** Where the "keep me on the television client" answer is remembered. */
export const STAY_ON_TV_KEY = "evey-stay-on-tv";

export function readStayOnTv(): boolean {
  try {
    return localStorage.getItem(STAY_ON_TV_KEY) === "1";
  } catch {
    return false;
  }
}
