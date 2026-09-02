import { useEffect, useMemo, useRef } from "react";
import { createLazyFileRoute, useNavigate } from "@tanstack/react-router";
import type { V1StreamInfo } from "@evey/api";
import { HLSPlayer } from "@/features/playback/HLSPlayer";
import { SyncPlayBar } from "@/features/syncplay/SyncPlayBar";
import { useSyncPlay } from "@/features/syncplay/useSyncPlay";
import { readGuestSession, clearGuestSession } from "@/features/syncplay/guestSession";

export const Route = createLazyFileRoute("/watch/$mediaId")({
  component: GuestWatch,
});

// GuestWatch is the PUBLIC (unauthenticated) "watch together" guest player. A
// guest reaches it after entering a room code on the login page: it plays the
// shared media with the guest stream token (streaming-only, forced transcode -
// the guest can't run the JWT-gated GetStreamInfo, so we skip the decision and
// let the server transcode), and joins the sync room via the guest ws path. It
// is intentionally bare: no back button, no download, no navigation - the guest
// is confined to this one title.
function GuestWatch() {
  const { mediaId } = Route.useParams();
  const navigate = useNavigate();
  const guest = useMemo(() => readGuestSession(mediaId), [mediaId]);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const joinedRef = useRef(false);

  const [syncState, syncControls] = useSyncPlay({
    guestToken: guest?.token,
    onPlay: (ms) => {
      const v = videoRef.current;
      if (!v) return;
      v.currentTime = ms / 1000;
      if (v.paused) void v.play().catch(() => {});
    },
    onPause: (ms) => {
      const v = videoRef.current;
      if (!v) return;
      v.currentTime = ms / 1000;
      v.pause();
    },
    onSeek: (ms) => {
      const v = videoRef.current;
      if (v) v.currentTime = ms / 1000;
    },
  });

  // A guest can't reach GetStreamInfo, so force the transcode path (always works)
  // with a client-side session id (the id is opaque server-side, only a metering
  // key). HLSPlayer only reads streamInfo.playMethod / directPlayUrl.
  const streamInfo = useMemo(() => ({ playMethod: "PLAY_METHOD_TRANSCODE" }) as V1StreamInfo, []);
  const sessionId = useMemo(() => crypto.randomUUID(), []);

  useEffect(() => {
    if (!guest) {
      void navigate({ to: "/login" });
      return;
    }
    // Join exactly once - guard against React StrictMode's double-invoke, whose
    // second connect() would close the first ws before it finished handshaking.
    if (joinedRef.current) return;
    joinedRef.current = true;
    syncControls.joinRoom(guest.roomId).catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps -- join once on mount for this guest session
  }, []);

  if (!guest) return null;

  return (
    <div className="fixed inset-0 bg-black z-overlay flex flex-col">
      <div className="flex-1 relative flex items-center justify-center">
        <HLSPlayer
          streamInfo={streamInfo}
          sessionId={sessionId}
          mediaId={mediaId}
          streamToken={guest.token}
          onStreamTokenRefresh={async () => guest.token}
          onVideoRef={(el) => { videoRef.current = el; }}
        />
      </div>
      {syncState.roomId && (
        <SyncPlayBar
          roomId={syncState.roomId}
          roomCode={syncState.roomCode}
          isHost={syncState.isHost}
          members={syncState.members}
          connected={syncState.connected}
          onLeave={() => {
            void syncControls.leaveRoom();
            clearGuestSession();
            void navigate({ to: "/login" });
          }}
          onResync={() => syncControls.sendResync(Math.floor((videoRef.current?.currentTime ?? 0) * 1000))}
          // The guest's own catch-up: asks the room where it is and seeks there
          // alone. Nobody else moves, which is what makes it usable by someone
          // who is not running the session.
          onCatchUp={() => syncControls.sendCatchUp()}
        />
      )}
    </div>
  );
}
