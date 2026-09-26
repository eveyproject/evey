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

// Whether this device has a keystore to remember a login in. Without one the
// login screen hides the checkbox instead of offering a promise it can't keep.
export async function canSaveCredentials(): Promise<boolean> {
  try {
    return await SecureStore.isAvailableAsync();
  } catch {
    return false;
  }
}

// Resolves false when the keystore refused the write, so the caller can say so.
export async function saveCredentials(username: string, password: string): Promise<boolean> {
  try {
    await SecureStore.setItemAsync(USER_KEY, username);
    await SecureStore.setItemAsync(PASS_KEY, password);
    return true;
  } catch {
    return false;
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
