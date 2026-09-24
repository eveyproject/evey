import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useAddToPlaylist, usePlaylistList } from "@evey/client";
import { FocusRoot, useBackHandler } from "../tv/focus";
import { Pressable, Spinner } from "./ui";
import { Icon } from "./icons";

/** The manual playlists a title can join, as the Android television lists them. */
export function AddToPlaylistSheet({ mediaItemId, mediaType, onClose }: { mediaItemId: string; mediaType?: string; onClose: () => void }) {
  const { t } = useTranslation();
  const { data: playlists, isLoading } = usePlaylistList();
  const addTo = useAddToPlaylist();
  const [added, setAdded] = useState<Set<string>>(new Set());
  useBackHandler(onClose);
  const manual = (playlists ?? []).filter((p) => p.type === "manual");

  const add = (playlistId: string) => {
    addTo.mutate(
      { playlistId, mediaItemId, mediaType },
      {
        // No onError: the shared mutation already says it.
        onSuccess: () => setAdded((prev) => new Set(prev).add(playlistId)),
      },
    );
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <FocusRoot autoFocus boundary className="sheet">
        <div onClick={(e) => e.stopPropagation()}>
          <h2 className="sheet-title">{t("media.detail.addToPlaylist")}</h2>
          {isLoading ? (
            <Spinner />
          ) : manual.length === 0 ? (
            <p className="sheet-empty">{t("common.noManualPlaylists", "No manual playlists available")}</p>
          ) : (
            <div className="sheet-list">
              {manual.map((p, i) => {
                const done = added.has(p.id);
                return (
                  <Pressable key={p.id} className="tv-card sheet-row" autoFocus={i === 0} label={p.name} onPress={() => { if (!done && !addTo.isPending) add(p.id); }}>
                    <span className="sheet-row-icon"><Icon name={done ? "checkmark" : "musical-note"} size={20} /></span>
                    <span className="sheet-row-name">{p.name}</span>
                    <span className={done ? "accent" : "muted"}>{done ? t("common.added", "Added") : "+"}</span>
                  </Pressable>
                );
              })}
            </div>
          )}
          <Pressable className="tv-card sheet-close" autoFocus={isLoading || manual.length === 0} onPress={onClose}>
            {t("media.detail.close", "Close")}
          </Pressable>
        </div>
      </FocusRoot>
    </div>
  );
}
