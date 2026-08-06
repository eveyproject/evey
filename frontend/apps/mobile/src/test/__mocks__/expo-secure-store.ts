// expo-secure-store pulls expo-modules-core's native EventEmitter at import
// time, which explodes under vitest (no Expo global). Screen tests that
// reach it transitively (media detail -> usePlayOnDevices) get this inert
// stub; tests that care about stored values vi.mock the module themselves.
export async function getItemAsync(_key: string): Promise<string | null> {
  return null;
}

export async function setItemAsync(_key: string, _value: string): Promise<void> {}

export async function deleteItemAsync(_key: string): Promise<void> {}
