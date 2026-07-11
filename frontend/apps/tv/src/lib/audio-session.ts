// Configure the global audio session so music keeps playing while the user
// navigates the TV UI. Uses expo-audio (the SDK 55 replacement for expo-av's
// removed ExponentAV native module). Best-effort: if the native module is
// absent (e.g. tvOS build without it), it no-ops silently.
let configured = false;

export async function configureAudioSession() {
  if (configured) return;
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const { setAudioModeAsync } = require("expo-audio") as {
      setAudioModeAsync: (mode: {
        playsInSilentMode?: boolean;
        shouldPlayInBackground?: boolean;
        interruptionMode?: "doNotMix" | "mixWithOthers" | "duckOthers";
        allowsRecording?: boolean;
      }) => Promise<void>;
    };
    await setAudioModeAsync({
      playsInSilentMode: true,
      shouldPlayInBackground: true,
      interruptionMode: "mixWithOthers",
      allowsRecording: false,
    });
    configured = true;
  } catch {
    // expo-audio not installed or native module missing - skip silently.
  }
}
