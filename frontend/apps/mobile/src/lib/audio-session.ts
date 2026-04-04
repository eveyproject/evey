import { Audio } from "expo-av";

let configured = false;

export async function configureAudioSession() {
  if (configured) return;
  try {
    await Audio.setAudioModeAsync({
      allowsRecordingIOS: false,
      staysActiveInBackground: true,
      playsInSilentModeIOS: true,
      shouldDuckAndroid: true,
      playThroughEarpieceAndroid: false,
    });
    configured = true;
  } catch (err) {
    console.warn("Failed to configure audio session:", err);
  }
}
