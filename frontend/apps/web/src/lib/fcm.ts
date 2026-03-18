import { initializeApp, getApps } from "firebase/app";
import { getMessaging, getToken, deleteToken } from "firebase/messaging";

let cachedToken: string | null = null;

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
    cachedToken = token;
  }
  return token || null;
}

export async function revokePushToken(): Promise<void> {
  if (!isFCMConfigured()) return;
  const app = getOrInitApp();
  const messaging = getMessaging(app);
  await deleteToken(messaging);
  cachedToken = null;
}

export function getStoredPushToken(): string | null {
  return cachedToken;
}
