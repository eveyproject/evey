import { useState, useRef } from "react";
import { View, Text, StyleSheet, Pressable, Alert, ActivityIndicator } from "react-native";
import { CameraView, useCameraPermissions } from "expo-camera";
import { Stack, useRouter } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../src/providers/ThemeProvider";
import { useUserServiceAuthorizeQuickConnect } from "@evey/api";

export default function QrScanScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const [permission, requestPermission] = useCameraPermissions();
  const [scanned, setScanned] = useState(false);
  const scannedRef = useRef(false);

  const authorizeQc = useUserServiceAuthorizeQuickConnect({
    mutation: {
      onSuccess: () => {
        router.back();
        Alert.alert(t("auth.settings.quickConnect.authorized"));
      },
      onError: () => {
        scannedRef.current = false;
        setScanned(false);
        Alert.alert(t("auth.settings.quickConnect.invalidCode"));
      },
    },
  });

  function handleBarCodeScanned({ data }: { data: string }) {
    if (scannedRef.current) return;
    scannedRef.current = true;
    setScanned(true);

    // Extract code from URL format: .../quickconnect?code=XXXX or raw code
    let code = data.trim();
    try {
      const url = new URL(data);
      const qp = url.searchParams.get("code");
      if (qp) code = qp;
    } catch {
      // not a URL, use raw value
    }

    // Extract last 6 chars if it's a long token
    if (code.length > 6) {
      code = code.slice(-6).toUpperCase();
    } else {
      code = code.toUpperCase();
    }

    authorizeQc.mutate({ data: { code } });
  }

  if (!permission) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Stack.Screen options={{ title: t("auth.settings.quickConnect.scanQr"), headerShown: true, headerStyle: { backgroundColor: theme.colors.surface }, headerTintColor: theme.colors.text }} />
        <ActivityIndicator color={theme.colors.accent} />
      </View>
    );
  }

  if (!permission.granted) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <Stack.Screen options={{ title: t("auth.settings.quickConnect.scanQr"), headerShown: true, headerStyle: { backgroundColor: theme.colors.surface }, headerTintColor: theme.colors.text }} />
        <Text style={{ color: theme.colors.textMuted, textAlign: "center", marginBottom: 16, paddingHorizontal: 24 }}>
          {t("auth.settings.quickConnect.cameraPermission")}
        </Text>
        <Pressable
          onPress={requestPermission}
          style={{ backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, paddingVertical: 10, paddingHorizontal: 24 }}
        >
          <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("auth.settings.quickConnect.grantPermission")}</Text>
        </Pressable>
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <Stack.Screen options={{ title: t("auth.settings.quickConnect.scanQr"), headerShown: true, headerStyle: { backgroundColor: theme.colors.surface }, headerTintColor: theme.colors.text }} />
      <CameraView
        style={StyleSheet.absoluteFillObject}
        facing="back"
        onBarcodeScanned={scanned ? undefined : handleBarCodeScanned}
        barcodeScannerSettings={{ barcodeTypes: ["qr"] }}
      />
      <View style={styles.overlay}>
        {/* fixed white: sits over the live camera feed, see rationale on `styles` below */}
        <View style={[styles.frame, { borderColor: scanned ? theme.colors.accent : "#fff" }]} />
        <Text style={styles.hint}>{t("auth.settings.quickConnect.scanHint")}</Text>
        {authorizeQc.isPending && (
          <ActivityIndicator color="#fff" size="large" style={{ marginTop: 16 }} />
        )}
        {scanned && !authorizeQc.isPending && (
          <Pressable
            onPress={() => { scannedRef.current = false; setScanned(false); }}
            style={{ backgroundColor: "rgba(255,255,255,0.2)", borderRadius: 8, paddingVertical: 10, paddingHorizontal: 24, marginTop: 16 }}
          >
            <Text style={{ color: "#fff", fontWeight: "600" }}>{t("common.retry")}</Text>
          </Pressable>
        )}
      </View>
    </View>
  );
}

// Colours below (black container fallback, white text/frame, black text
// shadow, translucent white retry button) are fixed on purpose: this screen
// is a full-bleed camera viewfinder, so the chrome sits over live video, not
// the app's themed background, and must stay legible regardless of theme.
const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#000" },
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  overlay: {
    ...StyleSheet.absoluteFillObject,
    justifyContent: "center",
    alignItems: "center",
  },
  frame: {
    width: 220,
    height: 220,
    borderWidth: 2,
    borderRadius: 12,
  },
  hint: {
    color: "#fff",
    fontSize: 14,
    marginTop: 20,
    textAlign: "center",
    paddingHorizontal: 32,
    textShadowColor: "rgba(0,0,0,0.8)",
    textShadowOffset: { width: 0, height: 1 },
    textShadowRadius: 4,
  },
});
