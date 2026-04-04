import { useCallback } from "react";
import { View, Text, FlatList, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useTranscodingServiceListTranscodeProfiles,
  useTranscodingServiceDeleteTranscodeProfile,
  getTranscodingServiceListTranscodeProfilesQueryKey,
} from "@evey/api";
import type { V1TranscodeProfile } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

export default function AdminTranscodingScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, refetch } = useTranscodingServiceListTranscodeProfiles();
  const profiles = data?.profiles ?? [];

  const deleteMut = useTranscodingServiceDeleteTranscodeProfile({
    mutation: {
      onSuccess: async () => {
        toast.success("Profile deleted");
        await qc.invalidateQueries({ queryKey: getTranscodingServiceListTranscodeProfilesQueryKey() });
      },
      onError: () => toast.error("Failed to delete"),
    },
  });

  const handleDelete = useCallback((p: V1TranscodeProfile) => {
    Alert.alert("Delete Profile", `Delete "${p.name}"?`, [
      { text: "Cancel", style: "cancel" },
      { text: "Delete", style: "destructive", onPress: () => p.id && deleteMut.mutate({ id: p.id }) },
    ]);
  }, [deleteMut]);

  const renderProfile = useCallback(({ item }: { item: V1TranscodeProfile }) => (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={{ flex: 1, gap: 2 }}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
          <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600" }}>{item.name}</Text>
          {item.isDefault && (
            <View style={{ backgroundColor: theme.colors.accent, borderRadius: 4, paddingHorizontal: 4, paddingVertical: 1 }}>
              <Text style={{ color: "#fff", fontSize: 9, fontWeight: "700" }}>DEFAULT</Text>
            </View>
          )}
        </View>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {[item.resolution, item.videoCodec, item.audioCodec, item.hwAccel !== "HW_ACCEL_TYPE_NONE" ? item.hwAccel?.replace("HW_ACCEL_TYPE_", "") : null].filter(Boolean).join(" · ")}
        </Text>
        {item.maxBitrateKbps ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{item.maxBitrateKbps} kbps</Text>
        ) : null}
      </View>
      <Pressable onPress={() => handleDelete(item)} hitSlop={8}>
        <Ionicons name="trash-outline" size={18} color="#ef4444" />
      </Pressable>
    </View>
  ), [theme, handleDelete]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: "Transcoding Profiles",
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />
      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <FlatList
          data={profiles}
          keyExtractor={(p) => p.id ?? ""}
          renderItem={renderProfile}
          contentContainerStyle={{ padding: 16, gap: 8 }}
          refreshing={false}
          onRefresh={refetch}
          ListEmptyComponent={
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Text style={{ color: theme.colors.textMuted }}>No transcode profiles</Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
});
