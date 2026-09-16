import { useEffect, useState } from "react";
import { View, Text, Pressable } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { getDownloadPath } from "../../lib/downloads";

interface Props {
  mediaId: string;
  /** The server answered 404: the title left the library, retrying is pointless. */
  gone: boolean;
  onRetry: () => void;
  onBack: () => void;
  onPlayLocal: () => void;
}

// The media page's failure state. Every part of that page comes from the
// server, so with the server out of reach it used to end on "retry" alone -
// while the film sat on the phone. A confirmed local copy adds the one
// action that works without a network; the player re-checks the parental
// gate itself, so this only has to know the bytes exist.
export function MediaLoadFailed({ mediaId, gone, onRetry, onBack, onPlayLocal }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [hasLocalCopy, setHasLocalCopy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setHasLocalCopy(false);
    getDownloadPath(mediaId)
      .then((p) => { if (!cancelled) setHasLocalCopy(!!p); })
      .catch(() => { /* a refused or unreadable index is just "no copy" */ });
    return () => { cancelled = true; };
  }, [mediaId]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg, justifyContent: "center", alignItems: "center", paddingHorizontal: theme.spacing.xl }}>
      <Text style={{ color: theme.colors.textMuted, marginBottom: 12, textAlign: "center" }}>
        {gone ? t("common.mediaGone") : t("common.loadError")}
      </Text>
      {!gone && hasLocalCopy && (
        <Pressable
          onPress={onPlayLocal}
          accessible
          accessibilityRole="button"
          accessibilityLabel={t("media.detail.playDownloadedCopy")}
          style={{ backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, paddingVertical: 10, paddingHorizontal: 20, marginBottom: theme.spacing.md }}
        >
          <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("media.detail.playDownloadedCopy")}</Text>
        </Pressable>
      )}
      <Pressable
        onPress={gone ? onBack : onRetry}
        accessible
        accessibilityRole="button"
        accessibilityLabel={gone ? t("common.back") : t("common.retry")}
      >
        <Text style={{ color: theme.colors.accent }}>{gone ? t("common.back") : t("common.retry")}</Text>
      </Pressable>
    </View>
  );
}
