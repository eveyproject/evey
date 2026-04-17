import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Select } from "@/components/ui/Select";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, useUserServiceUpdateProfile, getUserServiceListProfilesQueryKey } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { ISO_REGIONS } from "./regionList";

function regionOptions(locale: string): { value: string; label: string }[] {
  try {
    const sv = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf;
    const dynamic = typeof sv === "function" ? sv("region") : [];
    const codes = dynamic.length > 0 ? dynamic : Array.from(ISO_REGIONS);
    const names = new Intl.DisplayNames([locale], { type: "region" });
    return codes
      .filter((c) => /^[A-Z]{2}$/.test(c))
      .map((c) => ({ value: c, label: names.of(c) ?? c }))
      .sort((a, b) => a.label.localeCompare(b.label, locale));
  } catch {
    return Array.from(ISO_REGIONS).map((c) => ({ value: c, label: c }));
  }
}

export function RegionSettings() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const defaultProfile = profilesResp?.profiles?.find((p) => p.isDefault) ?? profilesResp?.profiles?.[0];
  const updateProfile = useUserServiceUpdateProfile();

  const options = useMemo(() => {
    return [{ value: "", label: t("common.region.auto") }, ...regionOptions(i18n.language)];
  }, [i18n.language, t]);

  const current = defaultProfile?.region ?? "";

  const handleChange = async (code: string) => {
    if (!defaultProfile?.id) return;
    try {
      await updateProfile.mutateAsync({ id: defaultProfile.id, data: { region: code } });
      if (user?.id) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
    } catch (err) { console.warn("region update failed", err); }
  };

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("common.region.title")}</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">{t("common.region.desc")}</p>
      </div>
      <Select
        label={t("common.region.title")}
        options={options}
        value={current}
        onChange={handleChange}
      />
    </div>
  );
}
