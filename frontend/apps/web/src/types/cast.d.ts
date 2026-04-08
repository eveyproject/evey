// Minimal type declarations for Google Cast Sender SDK
// https://developers.google.com/cast/docs/reference/web_sender

declare namespace cast {
  namespace framework {
    class CastContext {
      static getInstance(): CastContext;
      setOptions(options: { receiverApplicationId: string; autoJoinPolicy: unknown }): void;
      addEventListener(type: string, listener: (event: { castState?: string; sessionState?: string }) => void): void;
      requestSession(): Promise<void>;
      getCurrentSession(): CastSession | null;
    }

    class CastSession {
      getCastDevice(): { friendlyName: string } | null;
      loadMedia(request: chrome.cast.media.LoadRequest): Promise<void>;
      endSession(stopCasting: boolean): void;
    }

    class RemotePlayer {
      playerState: string | null;
      currentTime: number;
      duration: number;
      isPaused: boolean;
      isMediaLoaded: boolean;
      volumeLevel: number;
      isMuted: boolean;
      canSeek: boolean;
      canControlVolume: boolean;
    }

    class RemotePlayerController {
      constructor(player: RemotePlayer);
      addEventListener(type: string, listener: () => void): void;
      playOrPause(): void;
      seek(): void;
      setVolumeLevel(): void;
      muteOrUnmute(): void;
    }

    const CastContextEventType: {
      CAST_STATE_CHANGED: string;
      SESSION_STATE_CHANGED: string;
    };

    const RemotePlayerEventType: {
      PLAYER_STATE_CHANGED: string;
      CURRENT_TIME_CHANGED: string;
      DURATION_CHANGED: string;
      IS_MEDIA_LOADED_CHANGED: string;
      IS_PAUSED_CHANGED: string;
      VOLUME_LEVEL_CHANGED: string;
      IS_MUTED_CHANGED: string;
    };
  }
}

declare namespace chrome.cast {
  const AutoJoinPolicy: { ORIGIN_SCOPED: string };

  namespace media {
    class MediaInfo {
      constructor(contentId: string, contentType: string);
      streamType: string;
      hlsSegmentFormat: string;
      metadata: GenericMediaMetadata;
    }

    class GenericMediaMetadata {
      title: string;
    }

    class LoadRequest {
      constructor(mediaInfo: MediaInfo);
      autoplay: boolean;
    }

    const StreamType: { BUFFERED: string };
    const HlsSegmentFormat: { TS: string };
  }
}

interface Window {
  __onGCastApiAvailable?: (isAvailable: boolean) => void;
  cast?: typeof cast;
  chrome?: { cast?: typeof chrome.cast };
}
