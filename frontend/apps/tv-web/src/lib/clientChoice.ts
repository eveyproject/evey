/**
 * Which client a viewer meant, when both are served from the same host: the
 * television one under /tv, the web one at /. NOT a device test - measured in
 * the LG simulator, the webOS browser reports `pointer: fine` and `hover:
 * hover`, exactly like a computer, and a user-agent table needs a keeper.
 * So the viewer answers once and the answer is remembered.
 */

export const CLIENT_CHOICE_KEY = "evey-client-choice";

export type ClientChoice = "tv" | "web" | null;

/** A development host: nobody is asked anything while working on the client. */
const DEV_HOST = /^(localhost|127\.|\[?::1\]?|0\.0\.0\.0|.*\.local|192\.168\.|10\.|172\.(1[6-9]|2\d|3[01])\.)/i;

export interface ClientContext {
  /** file: for the packaged application, http(s): when served under /tv. */
  protocol: string;
  hostname: string;
  /** The path of the current address, /tv/ when the server serves it. */
  pathname: string;
  /** The query of the current address. */
  search: string;
  /** What this browser answered last time, if anything. */
  stored: ClientChoice;
}

// Only a served page asks, only once, and never when the address already
// says which client was meant. The packaged application IS the television
// client and has no web one beside it, so it never asks.
export function shouldAskWhichClient(ctx: ClientContext): boolean {
  if (ctx.protocol === "file:") return false;
  if (atWebRoot(ctx.pathname)) return false;
  if (DEV_HOST.test(ctx.hostname)) return false;
  if (choiceInAddress(ctx.search) !== null) return false;
  return ctx.stored === null;
}

/** Whether to leave for the web application straight away, with no question. */
export function shouldGoToWebClient(ctx: ClientContext): boolean {
  if (ctx.protocol === "file:") return false;
  if (atWebRoot(ctx.pathname)) return false;
  if (DEV_HOST.test(ctx.hostname)) return false;
  const fromAddress = choiceInAddress(ctx.search);
  if (fromAddress !== null) return fromAddress === "web";
  return ctx.stored === "web";
}

// When the television client is what the root serves, there is nothing next
// door and a redirect to "/" would reload itself for ever.
function atWebRoot(pathname: string): boolean {
  return pathname.replace(/index\.html$/, "") === "/";
}

/** `?client=tv` or `?client=web` on the address, which wins over the memory. */
export function choiceInAddress(search: string): ClientChoice {
  const match = /(?:^|[?&])client=(tv|web)(?:&|$)/.exec(search);
  return match ? (match[1] as ClientChoice) : null;
}

export function readChoice(): ClientChoice {
  try {
    const raw = localStorage.getItem(CLIENT_CHOICE_KEY);
    return raw === "tv" || raw === "web" ? raw : null;
  } catch {
    return null;
  }
}

export function rememberChoice(choice: Exclude<ClientChoice, null>): void {
  try {
    localStorage.setItem(CLIENT_CHOICE_KEY, choice);
  } catch {
    /* private window: the viewer will be asked again */
  }
}
