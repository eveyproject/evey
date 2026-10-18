import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";

// expo-local-authentication mock - mirroring useOnlineStatus.test.ts pattern
vi.mock("expo-local-authentication", () => ({
  AuthenticationType: {
    FINGERPRINT: 1,
    FACIAL_RECOGNITION: 2,
    IRIS: 3,
  },
  hasHardwareAsync: vi.fn(),
  isEnrolledAsync: vi.fn(),
  supportedAuthenticationTypesAsync: vi.fn(),
  authenticateAsync: vi.fn(),
}));

// expo-secure-store mock
vi.mock("expo-secure-store", () => ({
  getItemAsync: vi.fn(),
  setItemAsync: vi.fn(),
}));

import * as LocalAuthentication from "expo-local-authentication";
import * as SecureStore from "expo-secure-store";
import {
  useBiometricAvailable,
  useBiometricEnabled,
  authenticateBiometric,
} from "./useBiometric";

const mockHasHardware = vi.mocked(LocalAuthentication.hasHardwareAsync);
const mockIsEnrolled = vi.mocked(LocalAuthentication.isEnrolledAsync);
const mockSupportedTypes = vi.mocked(LocalAuthentication.supportedAuthenticationTypesAsync);
const mockAuthenticateAsync = vi.mocked(LocalAuthentication.authenticateAsync);
const mockGetItem = vi.mocked(SecureStore.getItemAsync);
const mockSetItem = vi.mocked(SecureStore.setItemAsync);

beforeEach(() => {
  vi.clearAllMocks();
});

// ---------------------------------------------------------------------------
// useBiometricAvailable
// ---------------------------------------------------------------------------

describe("useBiometricAvailable", () => {
  it("sets available=true when hardware present and biometric enrolled", async () => {
    mockHasHardware.mockResolvedValue(true);
    mockIsEnrolled.mockResolvedValue(true);
    mockSupportedTypes.mockResolvedValue([
      LocalAuthentication.AuthenticationType.FINGERPRINT,
    ]);

    const { result } = renderHook(() => useBiometricAvailable());
    await waitFor(() => expect(result.current.available).toBe(true));
  });

  it("sets available=false when hardware present but not enrolled", async () => {
    mockHasHardware.mockResolvedValue(true);
    mockIsEnrolled.mockResolvedValue(false);

    const { result } = renderHook(() => useBiometricAvailable());
    await waitFor(() => expect(result.current.available).toBe(false));
  });

  it("sets available=false when hardware absent", async () => {
    mockHasHardware.mockResolvedValue(false);
    mockIsEnrolled.mockResolvedValue(true);

    const { result } = renderHook(() => useBiometricAvailable());
    await waitFor(() => expect(result.current.available).toBe(false));
  });

  it("sets biometricType='face' when FACIAL_RECOGNITION is in supported types", async () => {
    mockHasHardware.mockResolvedValue(true);
    mockIsEnrolled.mockResolvedValue(true);
    mockSupportedTypes.mockResolvedValue([
      LocalAuthentication.AuthenticationType.FACIAL_RECOGNITION,
    ]);

    const { result } = renderHook(() => useBiometricAvailable());
    await waitFor(() => expect(result.current.biometricType).toBe("face"));
  });

  it("sets biometricType='fingerprint' when only FINGERPRINT is supported", async () => {
    mockHasHardware.mockResolvedValue(true);
    mockIsEnrolled.mockResolvedValue(true);
    mockSupportedTypes.mockResolvedValue([
      LocalAuthentication.AuthenticationType.FINGERPRINT,
    ]);

    const { result } = renderHook(() => useBiometricAvailable());
    await waitFor(() => expect(result.current.biometricType).toBe("fingerprint"));
  });

  it("leaves biometricType=null when no recognized type is supported", async () => {
    mockHasHardware.mockResolvedValue(true);
    mockIsEnrolled.mockResolvedValue(true);
    // Only IRIS (no FACIAL_RECOGNITION, no FINGERPRINT)
    mockSupportedTypes.mockResolvedValue([
      LocalAuthentication.AuthenticationType.IRIS,
    ]);

    const { result } = renderHook(() => useBiometricAvailable());
    await waitFor(() => expect(result.current.available).toBe(true));
    expect(result.current.biometricType).toBeNull();
  });

  it("sets available=false and swallows error when LocalAuthentication throws", async () => {
    mockHasHardware.mockRejectedValue(new Error("native module unavailable"));

    const { result } = renderHook(() => useBiometricAvailable());
    // Initial state is false; error in check() should keep it false
    await waitFor(() => expect(result.current.available).toBe(false));
    expect(result.current.biometricType).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// useBiometricEnabled
// ---------------------------------------------------------------------------

describe("useBiometricEnabled", () => {
  it("reads 'true' from SecureStore and sets enabled=true", async () => {
    mockGetItem.mockResolvedValue("true");

    const { result } = renderHook(() => useBiometricEnabled());
    await waitFor(() => expect(result.current.enabled).toBe(true));
    expect(mockGetItem).toHaveBeenCalledWith("evey-biometric-enabled");
  });

  it("leaves enabled=false when stored value is null", async () => {
    mockGetItem.mockResolvedValue(null);

    const { result } = renderHook(() => useBiometricEnabled());
    await waitFor(() => {
      // getItemAsync will have been called - enabled stays false
      expect(mockGetItem).toHaveBeenCalled();
    });
    expect(result.current.enabled).toBe(false);
  });

  it("leaves enabled=false and reports a keystore that refuses the read", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    mockGetItem.mockRejectedValue(new Error("keystore"));

    const { result } = renderHook(() => useBiometricEnabled());
    await waitFor(() => expect(warn).toHaveBeenCalled());
    expect(result.current.enabled).toBe(false);
    warn.mockRestore();
  });

  it("leaves enabled=false when stored value is not 'true'", async () => {
    mockGetItem.mockResolvedValue("false");

    const { result } = renderHook(() => useBiometricEnabled());
    await waitFor(() => expect(mockGetItem).toHaveBeenCalled());
    expect(result.current.enabled).toBe(false);
  });

  it("toggle(true) calls setItemAsync with 'true' and flips state to true", async () => {
    mockGetItem.mockResolvedValue("false");
    mockSetItem.mockResolvedValue(undefined);

    const { result } = renderHook(() => useBiometricEnabled());
    await waitFor(() => expect(mockGetItem).toHaveBeenCalled());

    await act(async () => {
      await result.current.toggle(true);
    });

    expect(mockSetItem).toHaveBeenCalledWith("evey-biometric-enabled", "true");
    expect(result.current.enabled).toBe(true);
  });

  it("toggle(false) calls setItemAsync with 'false' and flips state to false", async () => {
    mockGetItem.mockResolvedValue("true");
    mockSetItem.mockResolvedValue(undefined);

    const { result } = renderHook(() => useBiometricEnabled());
    await waitFor(() => expect(result.current.enabled).toBe(true));

    await act(async () => {
      await result.current.toggle(false);
    });

    expect(mockSetItem).toHaveBeenCalledWith("evey-biometric-enabled", "false");
    expect(result.current.enabled).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// authenticateBiometric (exported async helper)
// ---------------------------------------------------------------------------

describe("authenticateBiometric", () => {
  it("returns true when authenticateAsync succeeds", async () => {
    mockAuthenticateAsync.mockResolvedValue({ success: true, error: undefined });

    const result = await authenticateBiometric();
    expect(result).toBe(true);
    expect(mockAuthenticateAsync).toHaveBeenCalledWith({
      promptMessage: "Authenticate to continue",
      fallbackLabel: "Use password",
      cancelLabel: "Cancel",
    });
  });

  it("returns false when authenticateAsync returns success=false", async () => {
    mockAuthenticateAsync.mockResolvedValue({ success: false, error: "user_cancel" });

    const result = await authenticateBiometric();
    expect(result).toBe(false);
  });

  it("accepts a custom prompt message", async () => {
    mockAuthenticateAsync.mockResolvedValue({ success: true, error: undefined });

    await authenticateBiometric("Unlock to watch");
    expect(mockAuthenticateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ promptMessage: "Unlock to watch" }),
    );
  });

  it("returns false and swallows error when authenticateAsync throws", async () => {
    mockAuthenticateAsync.mockRejectedValue(new Error("hardware error"));

    const result = await authenticateBiometric();
    expect(result).toBe(false);
  });
});
