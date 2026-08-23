import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceUpdateProfile,
  getUserServiceListProfilesQueryKey,
  apiErrorMessage,
} from "@evey/api";
import type { V1Profile } from "@evey/api";
import { toast } from "@evey/store";

type UpdateProfileData = {
  name?: string;
  avatarPath?: string;
  maxRating?: string;
  maxBitrateKbps?: number;
  locale?: string;
  region?: string;
  isKids?: boolean;
  pin?: string;
  // -1 clears any persisted MaxAge, 0+ sets it. Only honored when isKids=true
  // server-side.
  maxAge?: number;
};

const MAX_AGE_OPTIONS = [
  { value: "", label: "" }, // resolved to t() at render time
  { value: "0", label: "0+" },
  { value: "6", label: "6+" },
  { value: "7", label: "7+" },
  { value: "10", label: "10+" },
  { value: "12", label: "12+" },
  { value: "13", label: "13+" },
  { value: "14", label: "14+" },
  { value: "15", label: "15+" },
  { value: "16", label: "16+" },
  { value: "17", label: "17+" },
  { value: "18", label: "18+" },
];
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { Checkbox } from "@/components/ui/Checkbox";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Select";
import { ISO_REGIONS } from "@/features/user/regionList";

interface EditProfileDialogProps {
  open: boolean;
  onClose: () => void;
  profile: V1Profile;
  userId: string;
  hasKidsProfile: boolean;
}

const LOCALE_OPTIONS = [
  { value: "", label: "" },
  { value: "en", label: "English" },
  { value: "fr", label: "Français" },
  { value: "de", label: "Deutsch" },
  { value: "es", label: "Español" },
  { value: "hi", label: "हिन्दी" },
  { value: "zh", label: "中文" },
];

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

export function EditProfileDialog({ open, onClose, profile, userId, hasKidsProfile }: EditProfileDialogProps) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const [name, setName] = useState(profile.name ?? "");
  const [isKids, setIsKids] = useState(!!profile.isKids);
  const [pin, setPin] = useState("");
  const [locale, setLocale] = useState(profile.locale ?? "");
  const [region, setRegion] = useState(profile.region ?? "");
  const [maxAge, setMaxAge] = useState(profile.maxAge != null ? String(profile.maxAge) : "");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName(profile.name ?? "");
      setIsKids(!!profile.isKids);
      setPin("");
      setLocale(profile.locale ?? "");
      setRegion(profile.region ?? "");
      setMaxAge(profile.maxAge != null ? String(profile.maxAge) : "");
      setError(null);
    }
  }, [open, profile]);

  const otherProfileHasKids = hasKidsProfile && !profile.isKids;
  const pinRequiredBecauseOfKids = otherProfileHasKids && !isKids;

  const regionOpts = useMemo(
    () => [{ value: "", label: t("common.region.auto", "Auto") }, ...regionOptions(i18n.language)],
    [i18n.language, t],
  );

  const updateProfile = useUserServiceUpdateProfile({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
        toast.success(t("auth.profile.updated", "Profile updated"));
        onClose();
      },
      onError: (err: Error) =>
        toast.error(apiErrorMessage(err, t, t("auth.profile.failedToUpdate", "Failed to update profile"))),
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (!name.trim()) {
      setError(t("auth.profile.fields.nameRequired", "Name is required"));
      return;
    }
    if (pin && pin.length !== 4) {
      setError(t("auth.profile.select.pinInvalid", "PIN must be 4 digits"));
      return;
    }
    if (!profile.id) return;
    const data: UpdateProfileData = {};
    if (name.trim() !== (profile.name ?? "")) data.name = name.trim();
    if (isKids !== !!profile.isKids) data.isKids = isKids;
    if (locale !== (profile.locale ?? "")) data.locale = locale;
    if (region !== (profile.region ?? "")) data.region = region;
    if (pin.length === 4) data.pin = pin;
    // Send maxAge only when the field is shown (kids profile). The backend
    // ignores it on non-kids anyway, but keeping it tied to the UI prevents
    // stale stored values from leaking back when the user toggles isKids.
    if (isKids) {
      const currentMaxAge = profile.maxAge != null ? String(profile.maxAge) : "";
      if (maxAge !== currentMaxAge) {
        if (maxAge === "") {
          data.maxAge = -1; // sentinel: clear
        } else {
          const n = parseInt(maxAge, 10);
          if (!Number.isNaN(n)) data.maxAge = n;
        }
      }
    }
    if (Object.keys(data).length === 0) {
      onClose();
      return;
    }
    updateProfile.mutate({ id: profile.id, data });
  }

  return (
    <Modal open={open} onClose={onClose} title={t("auth.profile.editProfile", "Edit Profile")}>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <Input
          label={t("auth.profile.fields.name", "Name")}
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
          autoFocus
        />
        <div className="flex flex-col gap-1">
          <Checkbox
            label={t("auth.profile.fields.isKids", "Kids profile")}
            checked={isKids}
            onChange={setIsKids}
          />
          <p className="text-xs text-[var(--text-muted)]">
            {t("auth.profile.fields.isKidsHelp", "Kids profiles only see age-appropriate content.")}
          </p>
        </div>
        {isKids && (
          <Select
            label={t("auth.profile.fields.maxAge", "Max age rating")}
            options={MAX_AGE_OPTIONS.map((o) => ({
              value: o.value,
              label: o.value === "" ? t("auth.settings.parental.defaultMaxAge", "Default (12+)") : o.label,
            }))}
            value={maxAge}
            onChange={setMaxAge}
          />
        )}
        <div className="flex flex-col gap-1">
          <Input
            label={t("auth.profile.fields.pin", "PIN (4 digits)")}
            type="password"
            inputMode="numeric"
            maxLength={4}
            pattern="\d{4}"
            placeholder="••••"
            value={pin}
            onChange={(e) => setPin(e.target.value.replace(/\D/g, "").slice(0, 4))}
            autoComplete="off"
          />
          <p className="text-xs text-[var(--text-muted)]">
            {pinRequiredBecauseOfKids
              ? t("auth.profile.fields.pinRequired", "A PIN is required because a kids profile exists.")
              : t("auth.profile.fields.pinKeep", "Leave blank to keep current PIN.")}
          </p>
        </div>
        <Select
          label={t("auth.profile.fields.locale", "Language")}
          options={LOCALE_OPTIONS}
          value={locale}
          onChange={setLocale}
        />
        <Select
          label={t("auth.profile.fields.region", "Region")}
          options={regionOpts}
          value={region}
          onChange={setRegion}
        />
        {error && (
          <p className="text-xs" style={{ color: "var(--color-danger)" }} role="alert">
            {error}
          </p>
        )}
        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" type="button" onClick={onClose}>
            {t("common.cancel", "Cancel")}
          </Button>
          <Button type="submit" loading={updateProfile.isPending}>
            {t("common.save", "Save")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
