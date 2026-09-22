// Shared implementation lives in @evey/api (langCodes.ts): the ISO 639 table
// and the audio-language matcher every player applies. This module keeps the
// path the screens and their tests import.
export { normalizeLangCode, pickPreferredStreamIndex } from "@evey/api";
