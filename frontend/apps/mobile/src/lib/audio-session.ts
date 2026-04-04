let configured = false;

export async function configureAudioSession() {
  if (configured) return;
  try {
    const { Audio } = require("expo-av");
    await Audio.setAudioModeAsync({
      allowsRecordingIOS: false,
      staysActiveInBackground: true,
      playsInSilentModeIOS: true,
      shouldDuckAndroid: true,
      playThroughEarpieceAndroid: false,
    });
    configured = true;
  } catch {
    // expo-av not available (Expo Go) — silently skip
  }
}
