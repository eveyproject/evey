import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranscodingServiceListTranscodeProfiles, getTranscodingServiceListTranscodeProfilesQueryKey } from "@evey/api";
import type {
  V1TranscodeProfile,
  V1HWAccelType,
} from "@evey/api/src/generated/transcoding.schemas";
import { V1HWAccelType as HWAccelType } from "@evey/api/src/generated/transcoding.schemas";
import { toast } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api/src/fetcher";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Modal } from "@/components/ui/Modal";

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
  // VideoToolbox not in proto enum — omitted
];

function hwAccelLabel(v: V1HWAccelType | undefined): string {
  return HW_ACCEL_OPTIONS.find((o) => o.value === v)?.label ?? "None";
}

function resolutionLabel(maxWidth?: number, maxHeight?: number): string {
  const match = RESOLUTION_OPTIONS.find(
    (r) => r.maxWidth === maxWidth && r.maxHeight === maxHeight,
  );
  return match ? match.label : maxHeight ? `${maxHeight}p` : "—";
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
        toast.success("Profile updated");
      } else {
        await eveyFetch({ url: `/api/v1/admin/transcoding/profiles`, method: "POST", data: body });
        toast.success("Profile created");
      }
      queryClient.invalidateQueries({ queryKey: getTranscodingServiceListTranscodeProfilesQueryKey() });
      handleClose();
    } catch {
      toast.error("Failed to save profile");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={handleClose}
      title={isEdit ? "Edit Profile" : "Create Profile"}
    >
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium" style={{ color: "var(--text)" }}>
            Name
          </label>
          <input
            type="text"
            required
            value={form.name}
            onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            className="px-3 py-2 text-sm rounded-[var(--radius-sm)]"
            style={{
              backgroundColor: "var(--surface-raised)",
              border: "1px solid var(--border)",
              color: "var(--text)",
            }}
            placeholder="e.g. High Quality 1080p"
          />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium" style={{ color: "var(--text)" }}>
              Max Resolution
            </label>
            <select
              value={form.resolution}
              onChange={(e) => setForm((f) => ({ ...f, resolution: e.target.value }))}
              className="px-3 py-2 text-sm rounded-[var(--radius-sm)]"
              style={{
                backgroundColor: "var(--surface-raised)",
                border: "1px solid var(--border)",
                color: "var(--text)",
              }}
            >
              {RESOLUTION_OPTIONS.map((r) => (
                <option key={r.label} value={r.label}>
                  {r.label}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium" style={{ color: "var(--text)" }}>
              Hardware Acceleration
            </label>
            <select
              value={form.hwAccel}
              onChange={(e) =>
                setForm((f) => ({ ...f, hwAccel: e.target.value as V1HWAccelType }))
              }
              className="px-3 py-2 text-sm rounded-[var(--radius-sm)]"
              style={{
                backgroundColor: "var(--surface-raised)",
                border: "1px solid var(--border)",
                color: "var(--text)",
              }}
            >
              {HW_ACCEL_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium" style={{ color: "var(--text)" }}>
              Video Codec
            </label>
            <select
              value={form.videoCodec}
              onChange={(e) => setForm((f) => ({ ...f, videoCodec: e.target.value }))}
              className="px-3 py-2 text-sm rounded-[var(--radius-sm)]"
              style={{
                backgroundColor: "var(--surface-raised)",
                border: "1px solid var(--border)",
                color: "var(--text)",
              }}
            >
              {VIDEO_CODEC_OPTIONS.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium" style={{ color: "var(--text)" }}>
              Audio Codec
            </label>
            <select
              value={form.audioCodec}
              onChange={(e) => setForm((f) => ({ ...f, audioCodec: e.target.value }))}
              className="px-3 py-2 text-sm rounded-[var(--radius-sm)]"
              style={{
                backgroundColor: "var(--surface-raised)",
                border: "1px solid var(--border)",
                color: "var(--text)",
              }}
            >
              {AUDIO_CODEC_OPTIONS.map((c) => (
                <option key={c} value={c}>
                  {c}
                </option>
              ))}
            </select>
          </div>
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium" style={{ color: "var(--text)" }}>
            Max Video Bitrate (kbps)
          </label>
          <input
            type="number"
            min={100}
            max={100000}
            value={form.maxBitrate}
            onChange={(e) => setForm((f) => ({ ...f, maxBitrate: e.target.value }))}
            className="px-3 py-2 text-sm rounded-[var(--radius-sm)]"
            style={{
              backgroundColor: "var(--surface-raised)",
              border: "1px solid var(--border)",
              color: "var(--text)",
            }}
          />
        </div>

        <label className="flex items-center gap-2 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={form.isDefault}
            onChange={(e) => setForm((f) => ({ ...f, isDefault: e.target.checked }))}
            className="w-4 h-4"
          />
          <span className="text-sm" style={{ color: "var(--text)" }}>
            Default profile
          </span>
        </label>

        <div className="flex justify-end gap-3 pt-2">
          <Button type="button" variant="secondary" onClick={handleClose}>
            Cancel
          </Button>
          <Button type="submit" loading={saving}>
            {isEdit ? "Save" : "Create"}
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
  const queryClient = useQueryClient();
  const [deleting, setDeleting] = useState(false);

  async function handleConfirm() {
    if (!profile?.id) return;
    setDeleting(true);
    try {
      await eveyFetch({ url: `/api/v1/admin/transcoding/profiles/${profile.id}`, method: "DELETE" });
      toast.success("Profile deleted");
      queryClient.invalidateQueries({ queryKey: getTranscodingServiceListTranscodeProfilesQueryKey() });
      onClose();
    } catch {
      toast.error("Failed to delete profile");
    } finally {
      setDeleting(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Delete Profile">
      <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
        Delete profile <strong style={{ color: "var(--text)" }}>{profile?.name}</strong>? This
        cannot be undone.
      </p>
      <div className="flex justify-end gap-3">
        <Button variant="secondary" onClick={onClose}>
          Cancel
        </Button>
        <Button variant="danger" loading={deleting} onClick={handleConfirm}>
          Delete
        </Button>
      </div>
    </Modal>
  );
}

function TranscodingProfilesPage() {
  const { data, isLoading } = useTranscodingServiceListTranscodeProfiles();
  const profiles = data?.profiles ?? [];

  const [showCreate, setShowCreate] = useState(false);
  const [editProfile, setEditProfile] = useState<V1TranscodeProfile | null>(null);
  const [deleteProfile, setDeleteProfile] = useState<V1TranscodeProfile | null>(null);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          Transcoding Profiles
        </h1>
        <Button onClick={() => setShowCreate(true)}>Create Profile</Button>
      </div>

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
                style={{
                  backgroundColor: "var(--surface)",
                  borderBottom: "1px solid var(--border)",
                }}
              >
                {["Name", "Resolution", "Video Codec", "Audio Codec", "Bitrate", "HW Accel", ""].map(
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
                    No transcoding profiles found.
                  </td>
                </tr>
              ) : (
                profiles.map((p) => (
                  <tr
                    key={p.id}
                    style={{ borderBottom: "1px solid var(--border)" }}
                  >
                    <td className="px-4 py-3" style={{ color: "var(--text)" }}>
                      <div className="flex items-center gap-2">
                        {p.name}
                        {p.isDefault && (
                          <Badge variant="success">Default</Badge>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3" style={{ color: "var(--text-muted)" }}>
                      {resolutionLabel(p.maxWidth, p.maxHeight)}
                    </td>
                    <td className="px-4 py-3" style={{ color: "var(--text-muted)" }}>
                      {p.videoCodec ?? "—"}
                    </td>
                    <td className="px-4 py-3" style={{ color: "var(--text-muted)" }}>
                      {p.audioCodec ?? "—"}
                    </td>
                    <td className="px-4 py-3 whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
                      {p.maxBitrate ? `${Number(p.maxBitrate).toLocaleString()} kbps` : "—"}
                    </td>
                    <td className="px-4 py-3" style={{ color: "var(--text-muted)" }}>
                      {hwAccelLabel(p.hwAccel)}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <Button
                          variant="secondary"
                          className="text-xs"
                          onClick={() => setEditProfile(p)}
                        >
                          Edit
                        </Button>
                        <Button
                          variant="danger"
                          className="text-xs"
                          onClick={() => setDeleteProfile(p)}
                        >
                          Delete
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
