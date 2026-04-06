import { useState, useCallback } from "react";
import { View, Text, FlatList, Pressable, TextInput, Alert, ActivityIndicator, StyleSheet, RefreshControl } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useTranscodingServiceListTranscodeProfiles,
  useTranscodingServiceCreateTranscodeProfile,
  useTranscodingServiceDeleteTranscodeProfile,
  getTranscodingServiceListTranscodeProfilesQueryKey,
} from "@evey/api";
import type { V1TranscodeProfile } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

const VIDEO_CODECS = ["h264", "h265", "av1", "vp9"];

export default function AdminTranscodingScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState("");
  const [codec, setCodec] = useState("h264");
  const [maxWidth, setMaxWidth] = useState("1920");
  const [maxHeight, setMaxHeight] = useState("1080");
  const [bitrate, setBitrate] = useState("4000");

  const { data, isLoading, refetch } = useTranscodingServiceListTranscodeProfiles();
  const profiles: V1TranscodeProfile[] = data?.profiles ?? [];

  const createProfile = useTranscodingServiceCreateTranscodeProfile({
    mutation: {
      onSuccess: () => {
        toast.success("Profile created");
        qc.invalidateQueries({ queryKey: getTranscodingServiceListTranscodeProfilesQueryKey() });
        resetForm();
      },
      onError: () => toast.error("Failed to create profile"),
    },
  });

  const deleteProfile = useTranscodingServiceDeleteTranscodeProfile({
    mutation: {
      onSuccess: () => {
        toast.success("Profile deleted");
        qc.invalidateQueries({ queryKey: getTranscodingServiceListTranscodeProfilesQueryKey() });
      },
      onError: () => toast.error("Failed to delete profile"),
    },
  });

  const resetForm = () => {
    setShowForm(false);
    setName("");
    setCodec("h264");
    setMaxWidth("1920");
    setMaxHeight("1080");
    setBitrate("4000");
  };

  const handleCreate = useCallback(async () => {
    if (!name.trim()) return;
    try {
      await createProfile.mutateAsync({
        data: {
          name: name.trim(),
          videoCodec: codec,
          maxWidth: parseInt(maxWidth, 10) || 1920,
          maxHeight: parseInt(maxHeight, 10) || 1080,
          videoBitrateKbps: parseInt(bitrate, 10) || 4000,
        },
      });
    } catch {
      // handled by onError
    }
  }, [name, codec, maxWidth, maxHeight, bitrate, createProfile]);

  const handleDelete = useCallback((profile: V1TranscodeProfile) => {
    Alert.alert("Delete Profile", `Delete "${profile.name}"?`, [
      { text: t("common.cancel"), style: "cancel" },
      { text: t("common.delete"), style: "destructive", onPress: () => deleteProfile.mutate({ id: profile.id! }) },
    ]);
  }, [deleteProfile, t]);

  const s = formStyles(theme);

  const onRefresh = useCallback(async () => {
    await refetch();
  }, [refetch]);

  const renderProfile = useCallback(({ item }: { item: V1TranscodeProfile }) => (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={{ flex: 1, gap: 4 }}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
          <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600" }}>{item.name}</Text>
          {item.isDefault && (
            <View style={{ backgroundColor: theme.colors.accent, borderRadius: 4, paddingHorizontal: 6, paddingVertical: 1 }}>
              <Text style={{ color: "#fff", fontSize: 10, fontWeight: "600" }}>DEFAULT</Text>
            </View>
          )}
        </View>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
          {item.videoCodec?.toUpperCase()} {item.maxWidth}x{item.maxHeight} {item.maxBitrate ? `${item.maxBitrate}kbps` : ""}
        </Text>
      </View>
      <Pressable onPress={() => handleDelete(item)} hitSlop={8} style={{ padding: 4 }}>
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
        headerRight: () => (
          <Pressable onPress={() => setShowForm(!showForm)} hitSlop={8} style={{ marginRight: 8 }}>
            <Ionicons name={showForm ? "close" : "add"} size={24} color={theme.colors.accent} />
          </Pressable>
        ),
      }} />

      {showForm && (
        <View style={[styles.formContainer, { backgroundColor: theme.colors.surface, borderBottomColor: theme.colors.border }]}>
          <Text style={s.label}>Name</Text>
          <TextInput value={name} onChangeText={setName} style={s.input} placeholderTextColor={theme.colors.textMuted} placeholder="1080p High" />

          <Text style={s.label}>Video Codec</Text>
          <View style={{ flexDirection: "row", gap: 8, flexWrap: "wrap" }}>
            {VIDEO_CODECS.map((c) => (
              <Pressable
                key={c}
                onPress={() => setCodec(c)}
                style={[styles.chip, {
                  backgroundColor: codec === c ? theme.colors.accent : theme.colors.surfaceRaised,
                  borderRadius: theme.radii.sm,
                }]}
              >
                <Text style={{ color: codec === c ? "#fff" : theme.colors.text, fontSize: 13, fontWeight: "500" }}>
                  {c.toUpperCase()}
                </Text>
              </Pressable>
            ))}
          </View>

          <View style={{ flexDirection: "row", gap: 12 }}>
            <View style={{ flex: 1 }}>
              <Text style={s.label}>Width</Text>
              <TextInput value={maxWidth} onChangeText={setMaxWidth} style={s.input} keyboardType="numeric" />
            </View>
            <View style={{ flex: 1 }}>
              <Text style={s.label}>Height</Text>
              <TextInput value={maxHeight} onChangeText={setMaxHeight} style={s.input} keyboardType="numeric" />
            </View>
          </View>

          <Text style={s.label}>Bitrate (kbps)</Text>
          <TextInput value={bitrate} onChangeText={setBitrate} style={s.input} keyboardType="numeric" />

          <Pressable
            onPress={handleCreate}
            disabled={createProfile.isPending || !name.trim()}
            style={[s.saveBtn, { opacity: createProfile.isPending || !name.trim() ? 0.5 : 1 }]}
          >
            <Text style={{ color: "#fff", fontWeight: "600", fontSize: 15 }}>
              {createProfile.isPending ? t("common.loading") : "Add Profile"}
            </Text>
          </Pressable>
        </View>
      )}

      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <FlatList
          data={profiles}
          keyExtractor={(p) => p.id!}
          renderItem={renderProfile}
          contentContainerStyle={{ padding: 16, gap: 8 }}
          refreshControl={<RefreshControl refreshing={false} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
          ListEmptyComponent={
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Ionicons name="swap-horizontal-outline" size={48} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>No transcoding profiles</Text>
            </View>
          }
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { flexDirection: "row", alignItems: "center", padding: 14, gap: 12 },
  formContainer: { padding: 16, borderBottomWidth: 1 },
  chip: { paddingHorizontal: 12, paddingVertical: 6 },
});

const formStyles = (t: ReturnType<typeof useTheme>) => StyleSheet.create({
  label: { color: t.colors.textMuted, fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginTop: 12, marginBottom: 6 },
  input: { backgroundColor: t.colors.surfaceRaised, color: t.colors.text, borderRadius: t.radii.md, paddingHorizontal: 14, paddingVertical: 12, fontSize: 15 },
  saveBtn: { backgroundColor: t.colors.accent, borderRadius: t.radii.md, paddingVertical: 14, alignItems: "center", marginTop: 16 },
});
