// Minimal expo-localization stub for vitest+jsdom environments.
// The real module imports expo-modules-core which expects a native
// ExpoGlobal.EventEmitter that isn't provided by jsdom.
export function getLocales() {
  return [{ languageCode: "en", languageTag: "en-US", regionCode: "US" }];
}

export function getCalendars() {
  return [{ calendar: "gregorian", timeZone: "UTC" }];
}
