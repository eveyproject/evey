// expo-secure-store has no JS-only implementation: importing the real module
// under vitest reaches for a native EventEmitter that does not exist and takes
// the whole suite down at collection time. Any test that needs to assert on
// stored values still mocks it locally with vi.mock.
const store = new Map<string, string>();

export async function getItemAsync(key: string): Promise<string | null> {
  return store.get(key) ?? null;
}

export async function setItemAsync(key: string, value: string): Promise<void> {
  store.set(key, value);
}

export async function deleteItemAsync(key: string): Promise<void> {
  store.delete(key);
}
