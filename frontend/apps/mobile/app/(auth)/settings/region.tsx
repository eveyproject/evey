import { useMemo, useState } from "react";
import { View, Text, TextInput, FlatList, Pressable, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  useUserServiceUpdateProfile,
  getUserServiceListProfilesQueryKey,
} from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

// Fallback list used when Hermes/JSC doesn't expose Intl.supportedValuesOf.
const ISO_REGIONS = [
  "AD","AE","AF","AG","AI","AL","AM","AO","AR","AS","AT","AU","AW","AX","AZ",
  "BA","BB","BD","BE","BF","BG","BH","BI","BJ","BL","BM","BN","BO","BQ","BR",
  "BS","BT","BV","BW","BY","BZ","CA","CC","CD","CF","CG","CH","CI","CK","CL",
  "CM","CN","CO","CR","CU","CV","CW","CX","CY","CZ","DE","DJ","DK","DM","DO",
  "DZ","EC","EE","EG","EH","ER","ES","ET","FI","FJ","FK","FM","FO","FR","GA",
  "GB","GD","GE","GF","GG","GH","GI","GL","GM","GN","GP","GQ","GR","GS","GT",
  "GU","GW","GY","HK","HM","HN","HR","HT","HU","ID","IE","IL","IM","IN","IO",
  "IQ","IR","IS","IT","JE","JM","JO","JP","KE","KG","KH","KI","KM","KN","KP",
  "KR","KW","KY","KZ","LA","LB","LC","LI","LK","LR","LS","LT","LU","LV","LY",
  "MA","MC","MD","ME","MF","MG","MH","MK","ML","MM","MN","MO","MP","MQ","MR",
  "MS","MT","MU","MV","MW","MX","MY","MZ","NA","NC","NE","NF","NG","NI","NL",
  "NO","NP","NR","NU","NZ","OM","PA","PE","PF","PG","PH","PK","PL","PM","PN",
  "PR","PS","PT","PW","PY","QA","RE","RO","RS","RU","RW","SA","SB","SC","SD",
  "SE","SG","SH","SI","SJ","SK","SL","SM","SN","SO","SR","SS","ST","SV","SX",
  "SY","SZ","TC","TD","TF","TG","TH","TJ","TK","TL","TM","TN","TO","TR","TT",
  "TV","TW","TZ","UA","UG","UM","US","UY","UZ","VA","VC","VE","VG","VI","VN",
  "VU","WF","WS","XK","YE","YT","ZA","ZM","ZW",
];

function regionOptions(locale: string): { code: string; label: string }[] {
  try {
    const sv = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf;
    const dynamic: string[] = typeof sv === "function" ? sv("region") : [];
    const codes = dynamic.length > 0 ? dynamic : ISO_REGIONS;
    const names = new Intl.DisplayNames([locale], { type: "region" });
    return codes
      .filter((c) => /^[A-Z]{2}$/.test(c))
      .map((c) => ({ code: c, label: names.of(c) ?? c }))
      .sort((a, b) => a.label.localeCompare(b.label, locale));
  } catch {
    return ISO_REGIONS.map((c) => ({ code: c, label: c }));
  }
}

export default function RegionScreen() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } } as never);
  const defaultProfile = profilesResp?.profiles?.find((p) => p.isDefault) ?? profilesResp?.profiles?.[0];
  const updateProfile = useUserServiceUpdateProfile({
    mutation: {
      onError: () => toast.error(t("common.region.updateFailed", "Failed to update region")),
    },
  });

  const [query, setQuery] = useState("");
  const options = useMemo(() => regionOptions(i18n.language), [i18n.language]);
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    return options.filter((o) => o.label.toLowerCase().includes(q) || o.code.toLowerCase().includes(q));
  }, [options, query]);

  const current = defaultProfile?.region ?? "";

  async function pick(code: string) {
    if (!defaultProfile?.id) return;
    try {
      await updateProfile.mutateAsync({ id: defaultProfile.id, data: { region: code } });
      if (user?.id) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
    } catch {
      // mutation onError already toasts
    }
  }

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{ title: t("common.region.title") }} />
      <TextInput
        placeholder={t("common.search") || "Search"}
        placeholderTextColor={theme.colors.textMuted}
        value={query}
        onChangeText={setQuery}
        style={{
          margin: theme.spacing.md,
          padding: theme.spacing.sm,
          borderRadius: theme.radii.md,
          backgroundColor: theme.colors.surface,
          color: theme.colors.text,
        }}
      />
      <FlatList
        data={[{ code: "", label: t("common.region.auto") }, ...filtered]}
        keyExtractor={(it) => it.code || "auto"}
        renderItem={({ item }) => (
          <Pressable
            onPress={() => pick(item.code)}
            style={({ pressed }) => [styles.row, { backgroundColor: pressed ? theme.colors.surfaceRaised : "transparent" }]}
          >
            <Text style={{ color: theme.colors.text, fontSize: 16 }}>{item.label}</Text>
            {current === item.code && (
              <Text style={{ color: theme.colors.accent, fontWeight: "700" }}>{"\u2713"}</Text>
            )}
          </Pressable>
        )}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", paddingHorizontal: 16, paddingVertical: 12, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: "rgba(255,255,255,0.08)" },
});
