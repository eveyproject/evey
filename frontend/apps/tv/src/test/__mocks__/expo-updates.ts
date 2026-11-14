// expo-updates is native-only. Under vitest it behaves like a development
// build: updates disabled, nothing pending.
export const isEnabled = false;
export const updateId: string | null = null;
export const isEmbeddedLaunch = true;

export function setUpdateURLAndRequestHeadersOverride(): void {}

export async function checkForUpdateAsync(): Promise<{ isAvailable: boolean }> {
  return { isAvailable: false };
}

export async function fetchUpdateAsync(): Promise<{ isNew: boolean }> {
  return { isNew: false };
}

export async function reloadAsync(): Promise<void> {}

export function useUpdates(): { isUpdatePending: boolean; downloadedUpdate?: { manifest?: unknown } } {
  return { isUpdatePending: false };
}
