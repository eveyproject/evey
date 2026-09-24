// expo-file-system/legacy needs the Expo native global at import time; any
// screen that transitively imports it (PluginSlot's bundle cache does)
// resolves here instead. Generous no-ops: tests that CARE about file-system
// behavior (downloads, audio pass) keep their richer per-file vi.mock, which
// takes precedence over this alias.
export const documentDirectory = "/data/user/0/app/files/";
export const cacheDirectory = "/data/user/0/app/cache/";

export async function getInfoAsync(): Promise<{ exists: boolean; size?: number }> {
  return { exists: false };
}
export async function readAsStringAsync(): Promise<string> {
  throw new Error("not found");
}
export async function writeAsStringAsync(): Promise<void> {}
export async function deleteAsync(): Promise<void> {}
export async function makeDirectoryAsync(): Promise<void> {}
export async function readDirectoryAsync(): Promise<string[]> {
  return [];
}
export async function moveAsync(): Promise<void> {}
export async function downloadAsync(): Promise<{ status: number }> {
  return { status: 200 };
}
