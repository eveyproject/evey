import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Select } from "@/components/ui/Select";
import { useUserServiceUpdateProfile, getUserServiceListProfilesQueryKey } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { ISO_REGIONS } from "./regionList";
import { useActiveProfile } from "@/hooks/useActiveProfile";
import { useDemoLock } from "@/hooks/useDemoMode";

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
  // region is per-profile: operate on the active profile (from the JWT), not
  // the default one - otherwise a non-default profile reads and writes the
  // wrong region.
  const { profile: activeProfile, userId } = useActiveProfile();
  const updateProfile = useUserServiceUpdateProfile();
  // The region is read by the server for every visitor of the shared
  // profile, so it has nowhere private to live on the demo: locked.
  const { lock } = useDemoLock();

  const options = useMemo(() => {
    return [{ value: "", label: t("common.region.auto") }, ...regionOptions(i18n.language)];
  }, [i18n.language, t]);

  const current = activeProfile?.region ?? "";

  const handleChange = async (code: string) => {
    if (!activeProfile?.id) return;
    try {
      await updateProfile.mutateAsync({ id: activeProfile.id, data: { region: code } });
      if (userId) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
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
        {...lock}
      />
    </div>
  );
}
