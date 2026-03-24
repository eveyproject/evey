import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useTranscodingServiceListTranscodeProfiles, getTranscodingServiceListTranscodeProfilesQueryKey } from "@evey/api";
import type { V1TranscodeProfile, V1HWAccelType } from "@evey/api";
import { V1HWAccelType as HWAccelType } from "@evey/api";
import { toast } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { PageHeader } from "@/components/ui/PageHeader";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { Checkbox } from "@/components/ui/Checkbox";

export const Route = createLazyFileRoute("/_auth/admin/transcoding-profiles")({
  component: TranscodingProfilesPage,
});

const RESOLUTION_OPTIONS = [
  { label: "480p", maxWidth: 854, maxHeight: 480 },
  { label: "720p", maxWidth: 1280, maxHeight: 720 },
  { label: "1080p", maxWidth: 1920, maxHeight: 1080 },
  { label: "4K", maxWidth: 3840, maxHeight: 2160 },
] as const;

const VIDEO_CODEC_OPTIONS = ["H.264", "H.265", "AV1"] as const;
const AUDIO_CODEC_OPTIONS = ["AAC", "AC3", "Opus"] as const;

const VIDEO_CODEC_MAP: Record<string, string> = {
  "H.264": "h264",
  "H.265": "hevc",
  "AV1": "av1",
};
const AUDIO_CODEC_MAP: Record<string, string> = {
  "AAC": "aac",
  "AC3": "ac3",
  "Opus": "opus",
};

const HW_ACCEL_OPTIONS: { label: string; value: V1HWAccelType }[] = [
  { label: "None", value: HWAccelType.HW_ACCEL_TYPE_NONE },
  { label: "VAAPI", value: HWAccelType.HW_ACCEL_TYPE_VAAPI },
  { label: "NVENC", value: HWAccelType.HW_ACCEL_TYPE_NVENC },
  { label: "QSV", value: HWAccelType.HW_ACCEL_TYPE_QSV },
];

function hwAccelLabel(v: V1HWAccelType | undefined): string {
  return HW_ACCEL_OPTIONS.find((o) => o.value === v)?.label ?? "None";
}

function resolutionLabel(maxWidth?: number, maxHeight?: number): string {
  const match = RESOLUTION_OPTIONS.find(
    (r) => r.maxWidth === maxWidth && r.maxHeight === maxHeight,
  );
  return match ? match.label : maxHeight ? `${maxHeight}p` : "\u2014";
}

interface ProfileFormState {
  name: string;
  resolution: string;
  videoCodec: string;
  audioCodec: string;
  maxBitrate: string;
  hwAccel: V1HWAccelType;
  isDefault: boolean;
}

const DEFAULT_FORM: ProfileFormState = {
  name: "",
  resolution: "1080p",
  videoCodec: "H.264",
  audioCodec: "AAC",
  maxBitrate: "8000",
  hwAccel: HWAccelType.HW_ACCEL_TYPE_NONE,
  isDefault: false,
};

interface ProfileDialogProps {
  open: boolean;
  profile: V1TranscodeProfile | null;
  onClose: () => void;
}

function ProfileDialog({ open, profile, onClose }: ProfileDialogProps) {
  const { t } = useTranslation();
  const isEdit = profile !== null;
  const queryClient = useQueryClient();

  const [form, setForm] = useState<ProfileFormState>(() => {
    if (!profile) return DEFAULT_FORM;
    return {
      name: profile.name ?? "",
      resolution: resolutionLabel(profile.maxWidth, profile.maxHeight),
      videoCodec: profile.videoCodec ?? "H.264",
      audioCodec: profile.audioCodec ?? "AAC",
      maxBitrate: profile.maxBitrate ? String(Number(profile.maxBitrate)) : "8000",
      hwAccel: profile.hwAccel ?? HWAccelType.HW_ACCEL_TYPE_NONE,
      isDefault: profile.isDefault ?? false,
    };
  });
  const [saving, setSaving] = useState(false);

  function handleClose() {
    setForm(DEFAULT_FORM);
    onClose();
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const res = RESOLUTION_OPTIONS.find((r) => r.label === form.resolution) ?? RESOLUTION_OPTIONS[2];
    const body = {
      name: form.name,
      videoCodec: VIDEO_CODEC_MAP[form.videoCodec] ?? form.videoCodec.toLowerCase(),
      audioCodec: AUDIO_CODEC_MAP[form.audioCodec] ?? form.audioCodec.toLowerCase(),
      container: "hls",
      maxWidth: res.maxWidth,
      maxHeight: res.maxHeight,
      videoBitrate: parseInt(form.maxBitrate) * 1000,
      audioBitrate: 192000,
      preset: "medium",
    };
    setSaving(true);
    try {
      if (isEdit && profile.id) {
        await eveyFetch({ url: `/api/v1/admin/transcoding/profiles/${profile.id}`, method: "PUT", data: body });
        toast.success(t("admin.transcodingProfiles.profileUpdated"));
      } else {
        await eveyFetch({ url: `/api/v1/admin/transcoding/profiles`, method: "POST", data: body });
        toast.success(t("admin.transcodingProfiles.profileCreated"));
      }
      queryClient.invalidateQueries({ queryKey: getTranscodingServiceListTranscodeProfilesQueryKey() });
      handleClose();
    } catch {
      toast.error(t("admin.transcodingProfiles.failedSave"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={handleClose}
      title={isEdit ? t("admin.transcodingProfiles.editProfile") : t("admin.transcodingProfiles.createProfile")}
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <Input
          label={t("admin.transcodingProfiles.form.name")}
          type="text"
          required
          value={form.name}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          placeholder="e.g. High Quality 1080p"
        />

        <div className="grid grid-cols-2 gap-4">
          <Select
            label={t("admin.transcodingProfiles.form.maxResolution")}
            value={form.resolution}
            onChange={(v) => setForm((f) => ({ ...f, resolution: v }))}
            options={RESOLUTION_OPTIONS.map((r) => ({ label: r.label, value: r.label }))}
          />

          <Select
            label={t("admin.transcodingProfiles.form.hwAccel")}
            value={form.hwAccel}
            onChange={(v) => setForm((f) => ({ ...f, hwAccel: v as V1HWAccelType }))}
            options={HW_ACCEL_OPTIONS.map((o) => ({ label: o.label, value: o.value }))}
          />

          <Select
            label={t("admin.transcodingProfiles.form.videoCodec")}
            value={form.videoCodec}
            onChange={(v) => setForm((f) => ({ ...f, videoCodec: v }))}
            options={VIDEO_CODEC_OPTIONS.map((c) => ({ label: c, value: c }))}
          />

          <Select
            label={t("admin.transcodingProfiles.form.audioCodec")}
            value={form.audioCodec}
            onChange={(v) => setForm((f) => ({ ...f, audioCodec: v }))}
            options={AUDIO_CODEC_OPTIONS.map((c) => ({ label: c, value: c }))}
          />
        </div>

        <Input
          label={t("admin.transcodingProfiles.form.maxBitrate")}
          type="number"
          min={100}
          max={100000}
          value={form.maxBitrate}
          onChange={(e) => setForm((f) => ({ ...f, maxBitrate: e.target.value }))}
        />

        <Checkbox
          label={t("admin.transcodingProfiles.form.defaultProfile")}
          checked={form.isDefault}
          onChange={(v) => setForm((f) => ({ ...f, isDefault: v }))}
        />

        <div className="flex justify-end gap-3 pt-2">
          <Button type="button" variant="secondary" onClick={handleClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" loading={saving}>
            {isEdit ? t("common.save") : t("common.create")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

interface DeleteDialogProps {
  open: boolean;
  profile: V1TranscodeProfile | null;
  onClose: () => void;
}

function DeleteDialog({ open, profile, onClose }: DeleteDialogProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [deleting, setDeleting] = useState(false);

  async function handleConfirm() {
    if (!profile?.id) return;
    setDeleting(true);
    try {
      await eveyFetch({ url: `/api/v1/admin/transcoding/profiles/${profile.id}`, method: "DELETE" });
      toast.success(t("admin.transcodingProfiles.profileDeleted"));
      queryClient.invalidateQueries({ queryKey: getTranscodingServiceListTranscodeProfilesQueryKey() });
      onClose();
    } catch {
      toast.error(t("admin.transcodingProfiles.failedDeleteProfile"));
    } finally {
      setDeleting(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("admin.transcodingProfiles.deleteProfile")}>
      <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
        {t("admin.transcodingProfiles.deleteConfirm", { name: profile?.name })}
      </p>
      <div className="flex justify-end gap-3">
        <Button variant="secondary" onClick={onClose}>
          {t("common.cancel")}
        </Button>
        <Button variant="danger" loading={deleting} onClick={handleConfirm}>
          {t("common.delete")}
        </Button>
      </div>
    </Modal>
  );
}

function TranscodingProfilesPage() {
  const { t } = useTranslation();
  const { data, isLoading } = useTranscodingServiceListTranscodeProfiles();
  const profiles = data?.profiles ?? [];

  const [showCreate, setShowCreate] = useState(false);
  const [editProfile, setEditProfile] = useState<V1TranscodeProfile | null>(null);
  const [deleteProfile, setDeleteProfile] = useState<V1TranscodeProfile | null>(null);

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.transcodingProfiles.title")} actions={<Button onClick={() => setShowCreate(true)}>{t("admin.transcodingProfiles.createProfile")}</Button>} />

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <div
          className="overflow-x-auto rounded-[var(--radius-lg)]"
          style={{ border: "1px solid var(--border)" }}
        >
          <table className="w-full text-sm">
            <thead>
              <tr
                style={{ backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }}
              >
                {[t("admin.transcodingProfiles.tableHeaders.name"), t("admin.transcodingProfiles.tableHeaders.resolution"), t("admin.transcodingProfiles.tableHeaders.videoCodec"), t("admin.transcodingProfiles.tableHeaders.audioCodec"), t("admin.transcodingProfiles.tableHeaders.bitrate"), t("admin.transcodingProfiles.tableHeaders.hwAccel"), ""].map(
                  (h) => (
                    <th
                      key={h}
                      className="px-4 py-2 text-left font-medium"
                      style={{ color: "var(--text-muted)" }}
                    >
                      {h}
                    </th>
                  ),
                )}
              </tr>
            </thead>
            <tbody>
              {profiles.length === 0 ? (
                <tr>
                  <td
                    colSpan={7}
                    className="px-4 py-8 text-center"
                    style={{ color: "var(--text-muted)" }}
                  >
                    {t("admin.transcodingProfiles.noProfiles")}
                  </td>
                </tr>
              ) : (
                profiles.map((p) => (
                  <tr key={p.id} style={{ borderBottom: "1px solid var(--border)" }}>
                    <td className="px-4 py-3" style={{ color: "var(--text)" }}>
                      <div className="flex items-center gap-2">
                        {p.name}
                        {p.isDefault && (
                          <Badge variant="success">{t("admin.transcodingProfiles.default")}</Badge>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3" style={{ color: "var(--text-muted)" }}>
                      {resolutionLabel(p.maxWidth, p.maxHeight)}
                    </td>
                    <td className="px-4 py-3" style={{ color: "var(--text-muted)" }}>
                      {p.videoCodec ?? "\u2014"}
                    </td>
                    <td className="px-4 py-3" style={{ color: "var(--text-muted)" }}>
                      {p.audioCodec ?? "\u2014"}
                    </td>
                    <td className="px-4 py-3 whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
                      {p.maxBitrate ? `${Number(p.maxBitrate).toLocaleString()} kbps` : "\u2014"}
                    </td>
                    <td className="px-4 py-3" style={{ color: "var(--text-muted)" }}>
                      {hwAccelLabel(p.hwAccel)}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <Button variant="secondary" className="text-xs" onClick={() => setEditProfile(p)}>
                          {t("common.edit")}
                        </Button>
                        <Button variant="danger" className="text-xs" onClick={() => setDeleteProfile(p)}>
                          {t("common.delete")}
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      )}

      <ProfileDialog
        open={showCreate || editProfile !== null}
        profile={editProfile}
        onClose={() => {
          setShowCreate(false);
          setEditProfile(null);
        }}
      />

      <DeleteDialog
        open={deleteProfile !== null}
        profile={deleteProfile}
        onClose={() => setDeleteProfile(null)}
      />
    </div>
  );
}
