import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceCreateProfile,
  getUserServiceListProfilesQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";

type CreateProfileData = {
  name?: string;
  avatarPath?: string;
  maxRating?: string;
  isKids?: boolean;
  pin?: string;
  locale?: string;
  region?: string;
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

interface CreateProfileDialogProps {
  open: boolean;
  onClose: () => void;
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

export function CreateProfileDialog({ open, onClose, userId, hasKidsProfile }: CreateProfileDialogProps) {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [isKids, setIsKids] = useState(false);
  const [pin, setPin] = useState("");
  const [locale, setLocale] = useState("");
  const [region, setRegion] = useState("");
  const [maxAge, setMaxAge] = useState("");
  const [error, setError] = useState<string | null>(null);

  const pinRequired = hasKidsProfile && !isKids;

  const regionOpts = useMemo(
    () => [{ value: "", label: t("common.region.auto", "Auto") }, ...regionOptions(i18n.language)],
    [i18n.language, t],
  );

  const createProfile = useUserServiceCreateProfile({
    mutation: {
      onSuccess: () => {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
        toast.success(t("auth.profile.created", "Profile created"));
        reset();
        onClose();
      },
      onError: (err: Error) =>
        toast.error(apiErrorMessage(err, t, t("auth.profile.failedToUpdate", "Failed to update profile"))),
    },
  });

  function reset() {
    setName("");
    setIsKids(false);
    setPin("");
    setLocale("");
    setRegion("");
    setMaxAge("");
    setError(null);
  }

  function handleClose() {
    reset();
    onClose();
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    if (!name.trim()) {
      setError(t("auth.profile.fields.nameRequired", "Name is required"));
      return;
    }
    if (pinRequired && pin.length !== 4) {
      setError(t("auth.profile.fields.pinRequired", "A PIN is required because a kids profile exists."));
      return;
    }
    const data: CreateProfileData = { name: name.trim() };
    if (isKids) {
      data.isKids = true;
      if (maxAge !== "") {
        const n = parseInt(maxAge, 10);
        if (!Number.isNaN(n)) data.maxAge = n;
      }
    }
    // Only attach a PIN to a non-kids profile: the PIN field is shown solely for
    // adult profiles (pinRequired = hasKidsProfile && !isKids). Without the !isKids
    // guard, a PIN typed before ticking "Kids profile" stays in state and leaks
    // onto the new kids profile - a PIN the user can no longer see or intended.
    if (!isKids && pin.length === 4) data.pin = pin;
    if (locale) data.locale = locale;
    if (region) data.region = region;
    createProfile.mutate({ userId, data });
  }

  return (
    <Modal open={open} onClose={handleClose} title={t("auth.profile.manage.createProfile", "Create Profile")}>
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
        {pinRequired && (
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
              required
            />
            <p className="text-xs text-[var(--text-muted)]">
              {t("auth.profile.fields.pinRequired", "A PIN is required because a kids profile exists.")}
            </p>
          </div>
        )}
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
          <Button variant="ghost" type="button" onClick={handleClose}>
            {t("common.cancel", "Cancel")}
          </Button>
          <Button type="submit" loading={createProfile.isPending}>
            {t("common.save", "Save")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
