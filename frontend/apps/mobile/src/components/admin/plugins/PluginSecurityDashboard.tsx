import { useState } from "react";
import { View, Text, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { useTheme } from "../../../providers/ThemeProvider";

// Port of the web features/admin/PluginSecurityDashboard: host sandbox status
// (which isolation is actually in force + why it degraded) and a per-plugin
// security snapshot (findings by severity, signed/unsigned, last error, source
// SHA). Shown even with zero plugins - that's when an admin most needs to learn
// the host can't isolate plugins.

interface SecurityFinding { category: string; severity: string; file: string; line: number; description?: string }
interface SignatureStatus { verified: boolean; verifiedBy?: string; error?: string }
interface PluginLastError { ts: string; message: string }
interface PluginSecuritySnapshot {
  id: string; name: string; version: string; sandboxMode: string; permissions: string[];
  findings: SecurityFinding[]; lastError?: PluginLastError; signature?: SignatureStatus;
  sourceSHA256: string; installedAt: string; lastStartedAt?: string; sizeBytes: number;
}
interface SandboxStatus { configured: string; effective: string; landlockKernel: boolean; launcherPresent: boolean; bubblewrapUsable: boolean }

function sandboxColor(mode: string, theme: ReturnType<typeof useTheme>): string {
  if (mode === "strict" || mode === "bubblewrap" || mode === "landlock") return theme.colors.success;
  if (mode === "basic") return theme.colors.warning;
  return theme.colors.textMuted;
}

function countBySeverity(findings: SecurityFinding[]) {
  return findings.reduce((acc, f) => {
    if (f.severity === "critical") acc.critical++;
    else if (f.severity === "warning") acc.warning++;
    else acc.info++;
    return acc;
  }, { critical: 0, warning: 0, info: 0 });
}

function Pill({ theme, label, color }: { theme: ReturnType<typeof useTheme>; label: string; color: string }) {
  return (
    <View style={{ paddingHorizontal: 6, paddingVertical: 2, borderRadius: theme.radii.sm, backgroundColor: color + "22" }}>
      <Text style={{ color, fontSize: 9, fontWeight: "700", textTransform: "uppercase" }}>{label}</Text>
    </View>
  );
}

function SandboxStatusCard({ sandbox, theme }: { sandbox: SandboxStatus; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const s = styles(theme);
  const degraded = sandbox.effective === "basic";
  const probes = [
    { key: "landlock", ok: sandbox.landlockKernel, label: t("admin.plugins.security.probeLandlock", "Landlock kernel") },
    { key: "launcher", ok: sandbox.launcherPresent, label: t("admin.plugins.security.probeLauncher", "Launcher") },
    { key: "bwrap", ok: sandbox.bubblewrapUsable, label: t("admin.plugins.security.probeBubblewrap", "bubblewrap") },
  ];
  return (
    <View style={[s.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={{ flexDirection: "row", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
        <Text style={s.hint}>{t("admin.plugins.security.sandboxHost", "Host sandbox")}</Text>
        <Pill theme={theme} label={sandbox.effective} color={sandboxColor(sandbox.effective, theme)} />
        {sandbox.configured !== sandbox.effective ? (
          <Text style={s.hint}>{t("admin.plugins.security.sandboxResolvedFrom", { mode: sandbox.configured })}</Text>
        ) : null}
      </View>
      {degraded && (
        <View style={{ marginTop: 8, padding: 10, borderRadius: theme.radii.sm, borderWidth: 1, borderColor: theme.colors.warning, backgroundColor: theme.colors.warning + "14", gap: 4 }}>
          <Text style={{ color: theme.colors.warning, fontSize: 12, fontWeight: "600" }}>{t("admin.plugins.security.sandboxBasicWarning", "Falling back to basic isolation.")}</Text>
          <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 12 }}>
            {probes.map((p) => (
              <Text key={p.key} style={{ fontSize: 11, color: theme.colors.textMuted }}>
                <Text style={{ color: p.ok ? theme.colors.success : theme.colors.error }}>{p.ok ? "✓" : "✗"}</Text> {p.label}
              </Text>
            ))}
          </View>
          <Text style={s.hint}>{t("admin.plugins.security.sandboxBasicHint", "Install the launcher or enable landlock/bubblewrap to isolate plugins.")}</Text>
        </View>
      )}
    </View>
  );
}

function SnapshotRow({ snap, theme }: { snap: PluginSecuritySnapshot; theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const s = styles(theme);
  const [expanded, setExpanded] = useState(false);
  const counts = countBySeverity(snap.findings ?? []);
  const hasFindings = (snap.findings ?? []).length > 0;
  const hasError = !!snap.lastError;

  return (
    <View style={[s.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, padding: 0 }]}>
      <Pressable onPress={() => setExpanded((v) => !v)} style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between", gap: 8, padding: 12 }}>
        <View style={{ flex: 1, minWidth: 0 }}>
          <Text style={{ color: theme.colors.text, fontWeight: "600" }} numberOfLines={1}>{snap.name}</Text>
          <Text style={s.hint}>v{snap.version}{snap.sourceSHA256 ? `  ${snap.sourceSHA256.slice(0, 8)}` : ""}</Text>
        </View>
        <View style={{ flexDirection: "row", alignItems: "center", gap: 6, flexWrap: "wrap", justifyContent: "flex-end" }}>
          <Pill theme={theme} label={snap.sandboxMode} color={sandboxColor(snap.sandboxMode, theme)} />
          {snap.permissions?.includes("subprocess") ? <Pill theme={theme} label={t("admin.plugin.subprocess", "subprocess")} color={theme.colors.warning} /> : null}
          {counts.critical > 0 ? (
            <Pill theme={theme} label={`${counts.critical} ${t("admin.plugins.security.critical", "critical")}`} color={theme.colors.error} />
          ) : counts.warning > 0 ? (
            <Pill theme={theme} label={`${counts.warning} ${t("admin.plugins.security.warnings", "warnings")}`} color={theme.colors.warning} />
          ) : (
            <Pill theme={theme} label={t("admin.plugins.security.clean", "clean")} color={theme.colors.success} />
          )}
          {snap.signature ? <Pill theme={theme} label={snap.signature.verified ? t("admin.plugins.security.signed", "signed") : t("admin.plugins.security.unsigned", "unsigned")} color={snap.signature.verified ? theme.colors.success : theme.colors.warning} /> : null}
          {hasError ? <Pill theme={theme} label={t("admin.plugins.security.errorBadge", "error")} color={theme.colors.error} /> : null}
          {(hasFindings || hasError) ? <Ionicons name={expanded ? "chevron-up" : "chevron-down"} size={16} color={theme.colors.textMuted} /> : null}
        </View>
      </Pressable>

      {expanded && (hasFindings || hasError) && (
        <View style={{ paddingHorizontal: 12, paddingBottom: 12, gap: 8, borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border }}>
          {hasFindings && (
            <View style={{ gap: 4, paddingTop: 8 }}>
              <Text style={[s.hint, { fontWeight: "600" }]}>{t("admin.plugins.security.findings", "Findings")}</Text>
              {snap.findings.map((f, i) => (
                <View key={i} style={{ flexDirection: "row", flexWrap: "wrap", alignItems: "center", gap: 6 }}>
                  <Text style={{ color: f.severity === "critical" ? theme.colors.error : theme.colors.warning, fontSize: 11, fontWeight: "700" }}>{f.severity.toUpperCase()}</Text>
                  <Text style={{ color: theme.colors.text, fontSize: 11, fontWeight: "600" }}>{f.category}</Text>
                  {f.description ? <Text style={{ color: theme.colors.textMuted, fontSize: 11 }} numberOfLines={1}>{f.description}</Text> : null}
                  {f.file ? <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{f.file}:{f.line}</Text> : null}
                </View>
              ))}
            </View>
          )}
          {hasError && snap.lastError && (
            <View style={{ padding: 10, borderRadius: theme.radii.sm, borderWidth: 1, borderColor: theme.colors.error, backgroundColor: theme.colors.error + "14" }}>
              <Text style={{ color: theme.colors.error, fontSize: 11, fontWeight: "600" }}>
                {t("admin.plugins.security.lastError", "Last error")} <Text style={{ color: theme.colors.textMuted, fontWeight: "400" }}>{new Date(snap.lastError.ts).toLocaleString()}</Text>
              </Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 2 }}>{snap.lastError.message}</Text>
            </View>
          )}
        </View>
      )}
    </View>
  );
}

export function PluginSecurityDashboard() {
  const theme = useTheme();
  const { t } = useTranslation();
  const s = styles(theme);
  const { data, isLoading } = useQuery({
    queryKey: ["/api/v1/admin/plugins/security-dashboard"],
    queryFn: () => eveyFetch<{ plugins: PluginSecuritySnapshot[]; sandbox?: SandboxStatus }>({ url: "/api/v1/admin/plugins/security-dashboard", method: "GET" }),
    staleTime: 30_000,
    refetchInterval: 60_000,
  });
  const snapshots = data?.plugins ?? [];

  return (
    <View style={{ gap: 8 }}>
      <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
        <Ionicons name="shield-checkmark-outline" size={18} color={theme.colors.textMuted} />
        <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "700" }}>{t("admin.plugins.security.title", "Plugin security")}</Text>
      </View>
      {isLoading ? (
        <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 16 }} />
      ) : (
        <>
          {data?.sandbox ? <SandboxStatusCard sandbox={data.sandbox} theme={theme} /> : null}
          {snapshots.length === 0 ? (
            <Text style={s.hint}>{t("admin.plugins.security.noData", "No plugin security data.")}</Text>
          ) : (
            snapshots.map((snap) => <SnapshotRow key={snap.id} snap={snap} theme={theme} />)
          )}
        </>
      )}
    </View>
  );
}

const styles = (theme: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    card: { padding: 12 },
    hint: { color: theme.colors.textMuted, fontSize: 12 },
  });
