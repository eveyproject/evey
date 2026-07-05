import * as SecureStore from "expo-secure-store";

// Opt-in "remember me" credentials, stored in the device keystore (SecureStore
// encrypts via Android Keystore / iOS Keychain). Kept separate from the token
// storage so a token purge (logout on another device, reuse-detection) never
// wipes the saved login.
const USER_KEY = "evey-saved-username";
const PASS_KEY = "evey-saved-password";

export async function getSavedCredentials(): Promise<{ username: string; password: string } | null> {
  try {
    const [username, password] = await Promise.all([
      SecureStore.getItemAsync(USER_KEY),
      SecureStore.getItemAsync(PASS_KEY),
    ]);
    if (!username || !password) return null;
    return { username, password };
  } catch {
    return null;
  }
}

export async function saveCredentials(username: string, password: string): Promise<void> {
  try {
    await SecureStore.setItemAsync(USER_KEY, username);
    await SecureStore.setItemAsync(PASS_KEY, password);
  } catch {
    /* no keystore available: silently skip, the checkbox just won't stick */
  }
}

export async function clearSavedCredentials(): Promise<void> {
  try {
    await Promise.all([
      SecureStore.deleteItemAsync(USER_KEY),
      SecureStore.deleteItemAsync(PASS_KEY),
    ]);
  } catch {
    /* ignore */
  }
}
