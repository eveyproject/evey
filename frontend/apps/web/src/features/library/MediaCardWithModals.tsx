import { memo, useState, useCallback } from "react";
import { MediaCard, type MediaCardProps } from "./MediaCard";
import { AddToPlaylistModal } from "../playlist/AddToPlaylistModal";
import { AddToCollectionModal } from "../collection/AddToCollectionModal";

export const MediaCardWithModals = memo(function MediaCardWithModals(props: MediaCardProps) {
  const [playlistTarget, setPlaylistTarget] = useState<string | null>(null);
  const [collectionTarget, setCollectionTarget] = useState<string | null>(null);

  const handleAddToPlaylist = useCallback((mediaId: string) => {
    setPlaylistTarget(mediaId);
  }, []);

  const handleAddToCollection = useCallback((mediaId: string) => {
    setCollectionTarget(mediaId);
  }, []);

  return (
    <>
      <MediaCard
        {...props}
        onRequestAddToPlaylist={handleAddToPlaylist}
        onRequestAddToCollection={handleAddToCollection}
      />
      {playlistTarget && (
        <AddToPlaylistModal
          open
          onClose={() => setPlaylistTarget(null)}
          mediaItemId={playlistTarget}
        />
      )}
      {collectionTarget && (
        <AddToCollectionModal
          open
          onClose={() => setCollectionTarget(null)}
          mediaItemIds={[collectionTarget]}
        />
      )}
    </>
  );
});
