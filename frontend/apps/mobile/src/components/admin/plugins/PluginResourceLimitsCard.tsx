import { useEffect, useState } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminPluginServiceGetPluginResourceLimits,
  useAdminPluginServiceUpdatePluginResourceLimits,
  getAdminPluginServiceGetPluginResourceLimitsQueryKey,
} from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../providers/ThemeProvider";

const SANDBOX_MODES = ["auto", "landlock", "bubblewrap", "strict", "basic"];

// Per-plugin resource caps the runtime's resource monitor enforces (mirror web
// PluginResourceLimits). Memory/PIDs/disk apply on the next sweep; cpu nice and
// sandbox mode apply to plugins (re)started afterwards.
export function PluginResourceLimitsCard() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const s = styles(theme);
  const { data, isLoading } = useAdminPluginServiceGetPluginResourceLimits();

  const [memoryMb, setMemoryMb] = useState("0");
  const [pidsMax, setPidsMax] = useState("0");
  const [diskMaxMb, setDiskMaxMb] = useState("0");
  const [cpuNice, setCpuNice] = useState("0");
  const [sandboxMode, setSandboxMode] = useState("auto");

  useEffect(() => {
    if (!data) return;
    setMemoryMb(String(data.memoryMb ?? 0));
    setPidsMax(String(data.pidsMax ?? 0));
    setDiskMaxMb(String(data.diskMaxMb ?? 0));
    setCpuNice(String(data.cpuNice ?? 0));
    setSandboxMode(data.sandboxMode || "auto");
  }, [data]);

  const update = useAdminPluginServiceUpdatePluginResourceLimits({
    mutation: {
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getAdminPluginServiceGetPluginResourceLimitsQueryKey() });
        toast.success(t("admin.plugins.resourceLimits.saved", "Resource limits saved"));
      },
      onError: () => toast.error(t("admin.plugins.resourceLimits.saveFailed", "Could not save resource limits")),
    },
  });

  const num = (v: string) => { const n = Number(v); return Number.isFinite(n) ? n : 0; };

  return (
    <View style={[s.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{t("admin.plugins.resourceLimits.title", "Resource limits")}</Text>
      <Text style={s.hint}>{t("admin.plugins.resourceLimits.description", "Caps the sandbox enforces on every plugin.")}</Text>

      {isLoading ? (
        <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 16 }} />
      ) : (
        <>
          <Field theme={theme} label={t("admin.plugins.resourceLimits.memoryMb", "Memory (MB)")} value={memoryMb} onChange={setMemoryMb} />
          <Field theme={theme} label={t("admin.plugins.resourceLimits.pidsMax", "Max PIDs")} value={pidsMax} onChange={setPidsMax} />
          <Field theme={theme} label={t("admin.plugins.resourceLimits.diskMaxMb", "Disk (MB)")} value={diskMaxMb} onChange={setDiskMaxMb} />
          <Field theme={theme} label={t("admin.plugins.resourceLimits.cpuNice", "CPU nice (0-19)")} value={cpuNice} onChange={setCpuNice} />

          <Text style={[s.label, { marginTop: 10 }]}>{t("admin.plugins.resourceLimits.sandboxMode", "Sandbox mode")}</Text>
          <View style={s.chipRow}>
            {SANDBOX_MODES.map((m) => {
              const active = sandboxMode === m;
              return (
                <Pressable key={m} onPress={() => setSandboxMode(m)} style={{ paddingHorizontal: 10, paddingVertical: 5, borderRadius: theme.radii.sm, borderWidth: 1, borderColor: active ? theme.colors.accent : theme.colors.border, backgroundColor: active ? theme.colors.accent : "transparent" }}>
                  <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 12, fontWeight: "600" }}>{m}</Text>
                </Pressable>
              );
            })}
          </View>

          <Pressable
            onPress={() => update.mutate({ data: { memoryMb: num(memoryMb), pidsMax: num(pidsMax), diskMaxMb: num(diskMaxMb), cpuNice: num(cpuNice), sandboxMode } })}
            disabled={update.isPending}
            style={[s.btn, { backgroundColor: theme.colors.accent, opacity: update.isPending ? 0.5 : 1 }]}
          >
            {update.isPending ? <ActivityIndicator size="small" color={theme.colors.onAccent} /> : <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>{t("admin.plugins.resourceLimits.save", "Save")}</Text>}
          </Pressable>
        </>
      )}
    </View>
  );
}

function Field({ theme, label, value, onChange }: { theme: ReturnType<typeof useTheme>; label: string; value: string; onChange: (v: string) => void }) {
  const s = styles(theme);
  return (
    <View style={{ marginTop: 8 }}>
      <Text style={s.label}>{label}</Text>
      <TextInput value={value} onChangeText={onChange} keyboardType="numeric" style={s.input} placeholderTextColor={theme.colors.textMuted} />
    </View>
  );
}

const styles = (theme: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    card: { padding: 14 },
    hint: { color: theme.colors.textMuted, fontSize: 12, marginTop: 2 },
    label: { color: theme.colors.textMuted, fontSize: 11, fontWeight: "600", textTransform: "uppercase", marginBottom: 4 },
    input: { backgroundColor: theme.colors.bg, color: theme.colors.text, borderRadius: theme.radii.md, paddingHorizontal: 12, paddingVertical: 10, fontSize: 15, borderWidth: StyleSheet.hairlineWidth, borderColor: theme.colors.border },
    chipRow: { flexDirection: "row", flexWrap: "wrap", gap: 6, marginBottom: 4 },
    btn: { borderRadius: theme.radii.md, paddingVertical: 12, alignItems: "center", marginTop: 14 },
  });
