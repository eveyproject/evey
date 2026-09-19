import { useEffect, useRef } from "react";
import { clearPlaybackState, onCastCommand, publishMetadata, publishPlaybackState, type CastCommand } from "@evey/cast-tv";

export interface CastConnectBridgeArgs {
  title: string;
  subtitle?: string;
  currentTime: number;
  duration: number;
  isPlaying: boolean;
  speed?: number;
  /** The player's own handlers. The bridge routes commands INTO them and
   *  never touches the playback itself. */
  onPlay: () => void;
  onPause: () => void;
  onStop: () => void;
  onSeekTo: (sec: number) => void;
  onNext?: () => void;
  onPrevious?: () => void;
}

// How often the projection is refreshed while nothing changes. The Cast
// senders extrapolate between updates from the state's position and rate, so
// a slow heartbeat is enough and a fast one is just wake-ups: what must be
// immediate is a TRANSITION (play/pause/seek), and those are pushed the frame
// they happen because the player's state changes.
const HEARTBEAT_MS = 1000;

/**
 * Projects the Evey TV player onto the Cast MediaSession, and routes the
 * commands that come back into the player's own handlers.
 *
 * The direction of authority matters and is the reason this hook is so thin:
 * the player decides, this reports. The MediaSession is what the television's
 * remote acts on and what the Cast SDK republishes to every connected sender,
 * so a phone and the web modal see the real position without knowing anything
 * about this app. Nothing here keeps its own copy of the playback state - the
 * moment it did, two answers to "where are we?" would exist.
 *
 * A no-op everywhere the native receiver is absent (tvOS, Expo Go, tests).
 */
export function useCastConnectBridge(args: CastConnectBridgeArgs): void {
  // Read-latest mirror: the command subscription is registered once, and must
  // not be torn down and rebuilt on every position tick.
  const handlers = useRef(args);
  handlers.current = args;

  useEffect(() => {
    const off = onCastCommand((cmd: CastCommand) => {
      const h = handlers.current;
      switch (cmd.kind) {
        case "play": h.onPlay(); break;
        case "pause": h.onPause(); break;
        case "stop": h.onStop(); break;
        case "next": h.onNext?.(); break;
        case "previous": h.onPrevious?.(); break;
        case "seek":
          if (typeof cmd.positionMs === "number") h.onSeekTo(cmd.positionMs / 1000);
          break;
      }
    });
    return off;
  }, []);

  useEffect(() => {
    publishMetadata(args.title, args.subtitle ?? "");
  }, [args.title, args.subtitle]);

  // Push on every transition, plus a slow heartbeat so a sender that joined
  // mid-film gets a fresh position without waiting for the next pause.
  useEffect(() => {
    const push = () => publishPlaybackState({
      positionSec: handlers.current.currentTime,
      durationSec: handlers.current.duration,
      playing: handlers.current.isPlaying,
      speed: handlers.current.speed ?? 1,
    });
    push();
    const id = setInterval(push, HEARTBEAT_MS);
    return () => clearInterval(id);
  }, [args.isPlaying, args.duration]);

  // Leaving the player: stop claiming a film is running, or the remote's play
  // key would resume something that is no longer on screen.
  useEffect(() => clearPlaybackState, []);
}
