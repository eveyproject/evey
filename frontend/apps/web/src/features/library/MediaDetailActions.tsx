import { useState } from "react";
import { Play, EyeOff, Eye, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "@tanstack/react-router";
import type { V1MediaItem, V1MediaPerson } from "@evey/api";
import {
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
} from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { ParentalPINDialog } from "@/components/ParentalPINDialog";
import { useParentalGate, markUnlocked } from "@/hooks/useParentalGate";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { DownloadButton } from "../playback/DownloadButton";
import { FavoriteButton } from "./FavoriteButton";
import { RatingButtons } from "./RatingButtons";
import { StarRating } from "./StarRating";
import { AddToPlaylistModal } from "../playlist/AddToPlaylistModal";
import { PlaySeriesButton } from "./MediaDetailSeries";
import { TrailerSection } from "./MediaDetailTrailer";
import { MetadataEditPanel } from "./MediaDetailEdit";

interface MediaDetailActionsProps {
  item: V1MediaItem;
  isSeries: boolean;
  isSeason: boolean;
  directors: V1MediaPerson[];
  writers: V1MediaPerson[];
  editing: boolean;
  setEditing: (v: boolean) => void;
  onMetadataUpdated?: () => void;
  streamToken: string | null;
}

export function MediaDetailActions({
  item,
  isSeries,
  isSeason,
  directors,
  writers,
  editing,
  setEditing,
  onMetadataUpdated,
  streamToken,
}: MediaDetailActionsProps) {
  const { t } = useTranslation();
  const [watched, setWatched] = useState(false);
  const [playlistModalOpen, setPlaylistModalOpen] = useState(false);
  const [pinDialogOpen, setPinDialogOpen] = useState(false);
  const [pendingPlayId, setPendingPlayId] = useState<string | null>(null);
  const profileId = useAuthStore((s) => s.profileId);
  const navigate = useNavigate();
  const { needsPin } = useParentalGate(item.officialRating);

  const markWatched = usePlaybackServiceMarkAsWatched({
    mutation: {
      onSuccess: () => { setWatched(true); toast.success(t("media.detail.markedWatched")); },
    },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => { setWatched(false); toast.success(t("media.detail.markedUnwatched")); },
    },
  });

  return (
    <div className="flex-1 space-y-4 min-w-0">
      {item.tagline && (
        <p className="text-base italic" style={{ color: "var(--text-muted)" }}>{item.tagline}</p>
      )}
      {item.overview && (
        <p className="text-sm leading-relaxed" style={{ color: "var(--text-muted)" }} data-testid="media-overview">{item.overview}</p>
      )}

      {directors.length > 0 && (
        <p className="text-sm" style={{ color: "var(--text-muted)" }}>
          <span className="font-medium" style={{ color: "var(--text)" }}>{t("media.detail.director")}</span>
          {directors.map((d, i) => (
            <span key={d.id ?? d.name}>
              {i > 0 && ", "}
              <Link to="/person/$personId" params={{ personId: d.id ?? "" }} className="hover:underline" style={{ color: "var(--accent)" }}>
                {d.name}
              </Link>
            </span>
          ))}
        </p>
      )}

      {writers.length > 0 && (
        <p className="text-sm" style={{ color: "var(--text-muted)" }}>
          <span className="font-medium" style={{ color: "var(--text)" }}>{t("media.detail.writer")}</span>
          {writers.map((w, i) => (
            <span key={w.id ?? w.name}>
              {i > 0 && ", "}
              <Link to="/person/$personId" params={{ personId: w.id ?? "" }} className="hover:underline" style={{ color: "var(--accent)" }}>
                {w.name}
              </Link>
            </span>
          ))}
        </p>
      )}

      {item.studios && item.studios.length > 0 && (
        <p className="text-sm" style={{ color: "var(--text-muted)" }}>
          <span className="font-medium" style={{ color: "var(--text)" }}>{t("media.detail.studio")}</span>
          {item.studios.join(", ")}
        </p>
      )}

      <TrailerSection trailers={item.trailers} />

      <div className="flex gap-3 flex-wrap pt-2" data-testid="media-actions">
        {item.id && isSeries && (
          <PlaySeriesButton
            seriesId={item.id}
            onPlay={(episodeId) => {
              if (needsPin()) {
                setPendingPlayId(episodeId);
                setPinDialogOpen(true);
              } else {
                void navigate({ to: "/player/$mediaId", params: { mediaId: episodeId } });
              }
            }}
          />
        )}
        {item.id && !isSeries && !isSeason && (
          <Button
            variant="primary"
            data-testid="play-button"
            onClick={() => {
              if (needsPin()) {
                setPendingPlayId(item.id!);
                setPinDialogOpen(true);
              } else {
                void navigate({ to: "/player/$mediaId", params: { mediaId: item.id! } });
              }
            }}
          >
            <Play size={16} fill="currentColor" />
            {t("media.detail.play")}
          </Button>
        )}
        {item.id && (
          watched ? (
            <Button variant="secondary" loading={markUnwatched.isPending} onClick={() => { if (profileId) markUnwatched.mutate({ mediaItemId: item.id!, params: { profileId } }); else toast.error(t("media.detail.noProfileSelected")); }} data-testid="mark-unwatched-btn">
              <EyeOff size={16} />
              {t("media.detail.markUnwatched")}
            </Button>
          ) : (
            <Button variant="secondary" loading={markWatched.isPending} onClick={() => { if (profileId) markWatched.mutate({ mediaItemId: item.id!, params: { profileId } }); else toast.error(t("media.detail.noProfileSelected")); }} data-testid="mark-watched-btn">
              <Eye size={16} />
              {t("media.detail.markWatched")}
            </Button>
          )
        )}
        {item.id && item.filePath && (
          <DownloadButton mediaId={item.id} title={item.title} streamToken={streamToken ?? undefined} />
        )}
        {item.id && <FavoriteButton mediaId={item.id} size="md" />}
        {item.id && <RatingButtons mediaId={item.id} />}
        {item.id && <StarRating mediaId={item.id} />}
        {item.id && (
          <Button variant="secondary" onClick={() => setPlaylistModalOpen(true)}>
            <Plus size={16} />
            {t("media.detail.addToPlaylist")}
          </Button>
        )}
      </div>
      {item.id && (
        <AddToPlaylistModal
          open={playlistModalOpen}
          onClose={() => setPlaylistModalOpen(false)}
          mediaItemId={item.id}
        />
      )}
      {profileId && (
        <ParentalPINDialog
          open={pinDialogOpen}
          onClose={() => { setPinDialogOpen(false); setPendingPlayId(null); }}
          onVerified={() => {
            if (profileId) markUnlocked(profileId);
            setPinDialogOpen(false);
            if (pendingPlayId) {
              void navigate({ to: "/player/$mediaId", params: { mediaId: pendingPlayId } });
            }
            setPendingPlayId(null);
          }}
          profileId={profileId}
        />
      )}
      {item.id && (
        <Modal open={editing} onClose={() => setEditing(false)} title={t("media.detail.editMetadata")}>
          <div className="overflow-y-auto" style={{ maxHeight: "70vh" }}>
            <MetadataEditPanel
              item={item}
              onSave={() => { setEditing(false); onMetadataUpdated?.(); }}
              onCancel={() => setEditing(false)}
            />
          </div>
        </Modal>
      )}
    </div>
  );
}
