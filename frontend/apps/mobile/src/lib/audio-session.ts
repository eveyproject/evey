let configured = false;

export async function configureAudioSession() {
  if (configured) return;
  try {
    // Check if native module exists before importing
    const { NativeModules } = require("react-native");
    if (!NativeModules.ExponentAV) return;
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
    // not available — skip
  }
}
