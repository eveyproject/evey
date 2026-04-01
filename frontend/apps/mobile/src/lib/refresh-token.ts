import * as SecureStore from "expo-secure-store";

const KEY = "evey-refresh-token";

export async function getRefreshToken(): Promise<string | null> {
  return SecureStore.getItemAsync(KEY);
}

export async function setRefreshToken(token: string): Promise<void> {
  await SecureStore.setItemAsync(KEY, token);
}

export async function clearRefreshToken(): Promise<void> {
  await SecureStore.deleteItemAsync(KEY);
}
