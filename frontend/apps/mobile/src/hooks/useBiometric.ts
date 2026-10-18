import { useState, useEffect, useCallback } from "react";
import * as LocalAuthentication from "expo-local-authentication";
import * as SecureStore from "expo-secure-store";

const BIOMETRIC_ENABLED_KEY = "evey-biometric-enabled";

export function useBiometricAvailable() {
  const [available, setAvailable] = useState(false);
  const [biometricType, setBiometricType] = useState<string | null>(null);

  useEffect(() => {
    async function check() {
      try {
        const compatible = await LocalAuthentication.hasHardwareAsync();
        const enrolled = await LocalAuthentication.isEnrolledAsync();
        setAvailable(compatible && enrolled);
        if (compatible && enrolled) {
          const types = await LocalAuthentication.supportedAuthenticationTypesAsync();
          if (types.includes(LocalAuthentication.AuthenticationType.FACIAL_RECOGNITION)) {
            setBiometricType("face");
          } else if (types.includes(LocalAuthentication.AuthenticationType.FINGERPRINT)) {
            setBiometricType("fingerprint");
          }
        }
      } catch {
        setAvailable(false);
      }
    }
    void check();
  }, []);

  return { available, biometricType };
}

export function useBiometricEnabled() {
  const [enabled, setEnabled] = useState(false);

  useEffect(() => {
    SecureStore.getItemAsync(BIOMETRIC_ENABLED_KEY)
      .then((v) => setEnabled(v === "true"))
      .catch((err) => console.warn("evey-biometric read", err));
  }, []);

  const toggle = useCallback(async (value: boolean) => {
    await SecureStore.setItemAsync(BIOMETRIC_ENABLED_KEY, value ? "true" : "false");
    setEnabled(value);
  }, []);

  return { enabled, toggle };
}

export async function authenticateBiometric(promptMessage = "Authenticate to continue"): Promise<boolean> {
  try {
    const result = await LocalAuthentication.authenticateAsync({
      promptMessage,
      fallbackLabel: "Use password",
      cancelLabel: "Cancel",
    });
    return result.success;
  } catch {
    return false;
  }
}
