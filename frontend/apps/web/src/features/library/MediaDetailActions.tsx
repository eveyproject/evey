import { useState } from "react";
import { formatTimeMs, itemLabel, type TrackPick } from "@evey/api";
import { invalidateWatchedState } from "@/lib/invalidateWatchedState";
import { RESUME_PROGRESS_QUERY } from "@evey/client";
import { Play, RotateCcw, EyeOff, Eye, Plus, SlidersHorizontal } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { useTranslation } from "react-i18next";
import { Link, useNavigate } from "@tanstack/react-router";
import type { V1MediaItem, V1MediaPerson } from "@evey/api";
import {
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
  usePlaybackServiceGetWatchProgress,
} from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { toast, useAuthStore } from "@evey/store";
import { ParentalPINDialog } from "@/components/ParentalPINDialog";
import { useParentalGate, markUnlocked } from "@/hooks/useParentalGate";
import { useStreamToken } from "@/hooks/useStreamToken";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { DownloadButton } from "../playback/DownloadButton";
import { CastMediaButton } from "../playback/CastMediaButton";
import { ShareLinkButton } from "./ShareLinkButton";
import { FavoriteButton } from "./FavoriteButton";
import { RatingButtons } from "./RatingButtons";
import { AddToPlaylistModal } from "../playlist/AddToPlaylistModal";
import { AddToCollectionModal } from "../collection/AddToCollectionModal";
import { PlaySeriesButton, PlaySeasonButton } from "./MediaDetailSeries";
import { TrailerSection } from "./MediaDetailTrailer";
import { MetadataEditPanel } from "./MediaDetailEdit";
import { MediaDetailTracks } from "./MediaDetailTracks";
import { hasMediaFile } from "./mediaDetailUtils";

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
  const [watchedOverride, setWatchedOverride] = useState<boolean | null>(null);
  const [playlistModalOpen, setPlaylistModalOpen] = useState(false);
  const [collectionModalOpen, setCollectionModalOpen] = useState(false);
  const [pinDialogOpen, setPinDialogOpen] = useState(false);
  const [pendingPlayId, setPendingPlayId] = useState<{ id: string; restart: boolean } | null>(null);
  // Audio/subtitle chosen before playback; empty = the player decides.
  const [trackPick, setTrackPick] = useState<TrackPick>({});
  const profileId = useAuthStore((s) => s.profileId);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const navigate = useNavigate();
  // Local mint for container (season/series) ZIP downloads: a media-scoped
  // stream token replaces the account JWT that used to ride in the URL.
  const { fetchStreamToken } = useStreamToken();

  // `restart` travels all the way to the player, PIN prompt included: the
  // choice must survive the detour, or entering a PIN would silently resume
  // the thing the viewer asked to watch from the start.
  const openPlayer = (episodeId: string, restart: boolean) => {
    // The track pick belongs to THIS file's streams: an episode launched from
    // a series page keeps the player's own resolution. `search` only when
    // there is something to say: an empty object would put a pointless `?`
    // on every play.
    const search = { ...(restart ? { restart: true as const } : {}), ...(episodeId === item.id ? trackPick : {}) };
    void navigate(
      Object.keys(search).length > 0
        ? { to: "/player/$mediaId", params: { mediaId: episodeId }, search }
        : { to: "/player/$mediaId", params: { mediaId: episodeId } },
    );
  };

  const handlePlay = (episodeId: string, restart = false) => {
    if (needsPin()) {
      setPendingPlayId({ id: episodeId, restart });
      setPinDialogOpen(true);
    } else {
      openPlayer(episodeId, restart);
    }
  };
  const qc = useQueryClient();
  const { needsPin } = useParentalGate(item.officialRating);

  const { data: progress } = usePlaybackServiceGetWatchProgress(item.id!, undefined, {
    query: { ...RESUME_PROGRESS_QUERY, enabled: !!item.id, retry: false },
  });
  const watched = watchedOverride ?? (progress?.played === true);
  const resumeMs = progress?.positionMs;
  const hasProgress = !!resumeMs && Number(resumeMs) > 0 && progress?.played !== true;

  const invalidateWatched = () => {
    invalidateWatchedState(qc, item.id);
  };

  const markWatched = usePlaybackServiceMarkAsWatched({
    mutation: {
      onSuccess: () => { setWatchedOverride(true); invalidateWatched(); toast.success(t("media.detail.markedWatched")); },
    },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => { setWatchedOverride(false); invalidateWatched(); toast.success(t("media.detail.markedUnwatched")); },
    },
  });

  return (
    <div className="flex-1 min-w-0 grid gap-x-10 gap-y-5 lg:grid-cols-[minmax(0,92ch)_minmax(190px,1fr)] items-start content-start">
      {/* Synopsis: no box. A bordered grey card gives the text the same visual
          weight as a form field, which is what made the page read as an admin
          panel. A hairline rule and a reading measure do the same containment
          job while letting the type carry itself. */}
      {(item.tagline || item.overview) && (
        <div className="max-w-[92ch] border-l border-[var(--border)] pl-6 py-0.5 space-y-4">
          {item.tagline && (
            <p className="text-2xl sm:text-3xl italic leading-snug text-[var(--text-secondary)] text-balance">{item.tagline}</p>
          )}
          {item.overview && (
            <p className="text-lg leading-[1.7] text-[var(--text-muted)]" data-testid="media-overview">{item.overview}</p>
          )}
        </div>
      )}

      {/* Reference column. Credits are looked UP, not read through - stacking
          them beside the synopsis fills the dead right-hand third and stops the
          narrative column from being interrupted by data. */}
      <aside className="lg:col-start-2 space-y-4 lg:border-l lg:border-[var(--border)] lg:pl-6">
      {directors.length > 0 && (
        <p className="text-sm text-[var(--text-muted)]">
          <span className="text-xs uppercase tracking-[0.14em] font-semibold text-[var(--text-secondary)] mr-2">{t("media.detail.director")}</span>
          {directors.map((d, i) => (
            <span key={d.id ?? d.name}>
              {i > 0 && ", "}
              <Link to="/person/$personId" params={{ personId: d.id ?? "" }} className="hover:underline text-[var(--accent)]">
                {d.name}
              </Link>
            </span>
          ))}
        </p>
      )}

      {writers.length > 0 && (
        <p className="text-sm text-[var(--text-muted)]">
          <span className="text-xs uppercase tracking-[0.14em] font-semibold text-[var(--text-secondary)] mr-2">{t("media.detail.writer")}</span>
          {writers.map((w, i) => (
            <span key={w.id ?? w.name}>
              {i > 0 && ", "}
              <Link to="/person/$personId" params={{ personId: w.id ?? "" }} className="hover:underline text-[var(--accent)]">
                {w.name}
              </Link>
            </span>
          ))}
        </p>
      )}

      {item.studios && item.studios.length > 0 && (
        <p className="text-sm text-[var(--text-muted)]">
          <span className="text-xs uppercase tracking-[0.14em] font-semibold text-[var(--text-secondary)] mr-2">{t("media.detail.studio")}</span>
          {item.studios.join(", ")}
        </p>
      )}

      </aside>

      <TrailerSection trailers={item.trailers} />

      {/* items-center, not the flex default of stretch: the icon-only controls
          carry their own square box, and stretching them to the row height is
          what deformed them into ovals. */}
      <div className="flex items-center gap-3 flex-wrap pt-2 lg:col-span-2" data-testid="media-actions">
        {item.id && isSeries && (
          <PlaySeriesButton seriesId={item.id} onPlay={handlePlay} />
        )}
        {item.id && isSeason && (
          <PlaySeasonButton seasonId={item.id} onPlay={handlePlay} />
        )}
        {item.id && !isSeries && !isSeason && (
          /* One control, two halves: the wide one resumes, the narrow one
             starts over. The divider is what makes them read as a single
             button instead of two rival Play buttons - with nothing to resume
             the second half is absent and the button is whole again. */
          <div className="flex">
            <Button
              variant="primary"
              size="lg"
              data-testid="play-button"
              className={hasProgress ? "rounded-r-none" : undefined}
              onClick={() => handlePlay(item.id!)}
            >
              <Play size={ICON.md} fill="currentColor" />
              {hasProgress ? `${t("media.detail.resume")} ${formatTimeMs(Number(resumeMs))}` : t("media.detail.play")}
            </Button>
            {hasProgress && (
              <Button
                variant="primary"
                size="lg"
                data-testid="play-from-start-button"
                aria-label={t("media.playFromStart")}
                title={t("media.playFromStart")}
                className="rounded-l-none border-l border-l-black/20 px-3"
                onClick={() => handlePlay(item.id!, true)}
              >
                <RotateCcw size={ICON.md} />
              </Button>
            )}
          </div>
        )}
        {/* Send it straight to the television, without opening the player
            here first. Playable items only - a series/season has no stream of
            its own to hand over. The parental PIN is asked BEFORE the stream
            leaves: a gate that only guards the local player would be no gate
            at all (the film would simply play on the TV instead). */}
        {item.id && !isSeries && !isSeason && hasMediaFile(item) && (
          <CastMediaButton
            mediaId={item.id}
            title={item.title}
            resumeMs={hasProgress ? Number(resumeMs) : 0}
            audioTrack={trackPick.audio}
            // Unlocking is all the dialog does here: the Cast picker needs
            // the user activation of a real tap, and the PIN round-trip has
            // spent the one that opened the dialog. The profile stays
            // unlocked, so the next press of this button goes straight
            // through - a silent refusal from Chrome would be worse.
            gate={() => {
              if (!needsPin()) return false;
              setPinDialogOpen(true);
              return true;
            }}
          />
        )}
        {/* Not offered for a series/season: the server's MarkAsWatched writes a
            single progress row on the id it is given and nothing cascades it to
            the episodes (no MediaMarkedWatched consumer descends the tree), so a
            "watched" series would leave every episode unwatched - a misleading
            button. Only real playable items (movie/episode) get the toggle.
            Kept in sync with apps/mobile. */}
        {item.id && !isSeries && !isSeason && (
          watched ? (
            <Button variant="secondary" size="lg" loading={markUnwatched.isPending} onClick={() => { if (profileId) markUnwatched.mutate({ mediaItemId: item.id!, params: { profileId } }); else toast.error(t("media.detail.noProfileSelected")); }} data-testid="mark-unwatched-btn">
              <EyeOff size={ICON.md} />
              {t("media.detail.markUnwatched")}
            </Button>
          ) : (
            <Button variant="secondary" size="lg" loading={markWatched.isPending} onClick={() => { if (profileId) markWatched.mutate({ mediaItemId: item.id!, params: { profileId } }); else toast.error(t("media.detail.noProfileSelected")); }} data-testid="mark-watched-btn">
              <Eye size={ICON.md} />
              {t("media.detail.markWatched")}
            </Button>
          )
        )}
        {item.id && hasMediaFile(item) && streamToken && (
          <DownloadButton mediaId={item.id} title={item.title} token={streamToken} size="lg" />
        )}
        {item.id && isSeason && (
          <DownloadButton
            mediaId={item.id}
            title={`${itemLabel(item, t) || "season"}.zip`}
            mintToken={() => fetchStreamToken(item.id!)}
            label={t("media.detail.downloadSeason")}
            size="lg"
          />
        )}
        {item.id && isSeries && (
          <DownloadButton
            mediaId={item.id}
            title={`${item.title ?? "series"}.zip`}
            mintToken={() => fetchStreamToken(item.id!)}
            label={t("media.detail.downloadSeries")}
            size="lg"
          />
        )}
        {item.id && item.filePath && isAdmin && <ShareLinkButton mediaId={item.id} />}
        {/* Segment editor. Unlike the share link just above, this is NOT
            restricted to playable items: the mobile client gates it on
            isAdmin && id alone, and a series/season page is a legitimate way
            in. Keep the two conditions in sync with apps/mobile. */}
        {item.id && isAdmin && (
          <Button
            variant="secondary"
            size="lg"
            data-testid="edit-segments-button"
            onClick={() => navigate({ to: "/admin/media/$mediaId", params: { mediaId: item.id! } })}
          >
            <SlidersHorizontal size={ICON.md} />
            {t("admin.segments.title")}
          </Button>
        )}
        {item.id && <FavoriteButton mediaId={item.id} size="lg" />}
        {item.id && <RatingButtons mediaId={item.id} />}
        {item.id && (
          <Button variant="secondary" size="lg" onClick={() => setPlaylistModalOpen(true)}>
            <Plus size={ICON.md} />
            {t("media.detail.addToPlaylist")}
          </Button>
        )}
        {/* Grouping a title into a saga is thought of here, on the title's own
            page, not from a grid card - but only the card menu offered it.
            Admin-gated like the RPC (AddCollectionItem is RoleAdmin). */}
        {item.id && isAdmin && (
          <Button
            variant="secondary"
            size="lg"
            data-testid="add-to-collection-button"
            onClick={() => setCollectionModalOpen(true)}
          >
            <Plus size={ICON.md} />
            {t("media.contextMenu.addToCollection")}
          </Button>
        )}
      </div>
      {item.id && !isSeries && !isSeason && item.streams && (
        <MediaDetailTracks streams={item.streams} pick={trackPick} onChange={setTrackPick} />
      )}
      {item.id && (
        <AddToPlaylistModal
          open={playlistModalOpen}
          onClose={() => setPlaylistModalOpen(false)}
          mediaItemId={item.id}
        />
      )}
      {item.id && isAdmin && (
        <AddToCollectionModal
          open={collectionModalOpen}
          onClose={() => setCollectionModalOpen(false)}
          mediaItemIds={[item.id]}
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
              openPlayer(pendingPlayId.id, pendingPlayId.restart);
            }
            setPendingPlayId(null);
          }}
          profileId={profileId}
        />
      )}
      {/* Defense in depth: re-check isAdmin at the mount point, not just at the
          toggle button in MediaDetailHero, so a stale editing=true can never
          render the editor for a non-admin (e.g. after losing admin mid-session). */}
      {item.id && isAdmin && (
        <Modal open={editing} onClose={() => setEditing(false)} title={t("media.detail.editMetadata")} size="4xl">
          <div className="overflow-y-auto max-h-[76vh]">
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
