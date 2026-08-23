// What to SHOW a viewer when an API call fails.
//
// The server has two ways of saying why: a machine `code` on the JSON body, or
// a `message` that is itself a catalogue key ("errors.auth.invalid_pin"). Both
// exist to keep English prose off a French screen. The fetcher already lifts
// them onto the thrown Error (message + errorCode) - what was missing is a
// SINGLE answer to "now render it", and the three clients had three:
//
//   web parental screen : t(raw) only when raw started with "errors."
//   mobile parental     : Alert.alert(..., err.message)  - raw, always
//   mobile settings     : err.message || "Update failed" - raw, English default
//
// So the rate-limit refusal, which has been an i18n KEY on the server for
// months, reached the mobile parental screen as the literal string
// "errors.auth.too_many_attempts". That is the beta.38 raw-key regression,
// still live on one screen. One function, one answer.
import type { Translate } from "./displayTitle";

// A key is anything the catalogue could resolve. Restricted to the "errors."
// namespace on purpose: it is the only prefix the server promises, and a
// blanket "does it contain a dot" would send real sentences through t().
const ERROR_KEY_PREFIX = "errors.";

export interface ApiErrorLike {
  message?: string;
  errorCode?: string;
}

// apiErrorMessage renders a thrown API error into viewer-facing text.
// Precedence: machine code, then a key-shaped message, then the server's prose,
// then the caller's own fallback. Never returns a bare catalogue key: t() is
// always given the raw string as its own default, so an unknown key degrades to
// the server's text rather than to "errors.something" on screen.
export function apiErrorMessage(err: ApiErrorLike | null | undefined, t: Translate, fallback: string): string {
  const code = err?.errorCode ?? "";
  if (code.startsWith(ERROR_KEY_PREFIX)) {
    const out = t(code, { defaultValue: code });
    if (out && out !== code) return out;
  }
  const raw = err?.message ?? "";
  if (raw.startsWith(ERROR_KEY_PREFIX)) {
    const out = t(raw, { defaultValue: raw });
    // An unresolved key must never reach a screen - fall through to the
    // fallback rather than print "errors.auth.invalid_pin" at the viewer.
    if (out && out !== raw) return out;
    return fallback;
  }
  return raw || fallback;
}
