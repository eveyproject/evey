import { initializeApp, getApps } from "firebase/app";
import { getMessaging, getToken, deleteToken } from "firebase/messaging";

const TOKEN_STORAGE = "evey_fcm_token";

function getFirebaseConfig() {
  return {
    apiKey: import.meta.env.VITE_FIREBASE_API_KEY,
    projectId: import.meta.env.VITE_FIREBASE_PROJECT_ID,
    messagingSenderId: import.meta.env.VITE_FIREBASE_MESSAGING_SENDER_ID,
    appId: import.meta.env.VITE_FIREBASE_APP_ID,
  };
}

export function isFCMConfigured(): boolean {
  const c = getFirebaseConfig();
  return !!(c.apiKey && c.projectId && c.messagingSenderId && c.appId);
}

function getOrInitApp() {
  const config = getFirebaseConfig();
  const existing = getApps().find((a) => a.name === "[DEFAULT]");
  if (existing) return existing;
  return initializeApp({
    apiKey: config.apiKey,
    projectId: config.projectId,
    messagingSenderId: config.messagingSenderId,
    appId: config.appId,
  });
}

export async function requestPushToken(vapidKey: string): Promise<string | null> {
  if (!isFCMConfigured()) return null;

  const permission = await Notification.requestPermission();
  if (permission !== "granted") return null;

  const app = getOrInitApp();
  const messaging = getMessaging(app);
  const token = await getToken(messaging, { vapidKey });
  if (token) {
    localStorage.setItem(TOKEN_STORAGE, token);
  }
  return token || null;
}

export async function revokePushToken(): Promise<void> {
  if (!isFCMConfigured()) return;
  const app = getOrInitApp();
  const messaging = getMessaging(app);
  await deleteToken(messaging);
  localStorage.removeItem(TOKEN_STORAGE);
}

export function getStoredPushToken(): string | null {
  return localStorage.getItem(TOKEN_STORAGE);
}
