import { useTranslation } from "react-i18next";
import {
  SUBTITLE_BG_COLORS,
  SUBTITLE_TEXT_COLORS,
  subtitleBgRgba,
  useSubtitleStyleSetting,
} from "@evey/client";

const LABEL = "text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]";

// Per-profile: how subtitles look. The player's own menu writes the same
// profile preference, so the choice follows the viewer across devices and can
// be set here without starting a title.
export function SubtitleStyleSettings() {
  const { t } = useTranslation();
  const { style, update, ready } = useSubtitleStyleSetting();

  return (
    <div className="flex flex-col gap-4" data-testid="subtitle-style-settings">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.playback.subtitleStyleTitle")}</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">{t("auth.settings.playback.subtitleStyleDesc")}</p>
      </div>

      {/* A stand-in for a video frame: dark whatever the theme, since that is what subtitles sit on. */}
      <div
        className={`flex justify-center rounded-[var(--radius-md)] bg-neutral-900 px-4 py-6 min-h-28 ${style.position === "top" ? "items-start" : "items-end"}`}
        aria-hidden="true"
      >
        <span
          data-testid="subtitle-style-preview"
          className="rounded px-2 py-0.5 text-center"
          style={{
            color: style.color,
            fontSize: `${(16 * style.fontSize) / 100}px`,
            lineHeight: 1.3,
            backgroundColor: subtitleBgRgba(style.bgColor, style.bgOpacity),
            textShadow: style.shadow ? "0 0 4px #000, 0 1px 2px #000" : undefined,
          }}
        >
          {t("auth.settings.playback.subtitleStylePreview")}
        </span>
      </div>

      <fieldset disabled={!ready} className="grid gap-4 sm:grid-cols-2 min-w-0">
        <div>
          <div className="flex items-center justify-between mb-1">
            <span className={LABEL}>{t("player.subtitleStyle.size")}</span>
            <span className="text-xs text-[var(--text-muted)]">{style.fontSize}%</span>
          </div>
          <input
            type="range"
            min={50}
            max={200}
            step={10}
            value={style.fontSize}
            onChange={(e) => update({ fontSize: Number(e.target.value) })}
            className="w-full accent-[var(--accent)]"
            aria-label={t("player.subtitleStyle.size")}
          />
        </div>

        <div>
          <span className={`${LABEL} block mb-2`}>{t("player.subtitleStyle.textColor")}</span>
          <div className="flex gap-2">
            {SUBTITLE_TEXT_COLORS.map((c) => (
              <button
                key={c.value}
                type="button"
                title={t(c.labelKey)}
                aria-label={t(c.labelKey)}
                aria-pressed={style.color === c.value}
                onClick={() => update({ color: c.value })}
                className="w-6 h-6 rounded-full border border-[var(--border)]"
                style={{
                  backgroundColor: c.value,
                  outline: style.color === c.value ? "2px solid var(--accent)" : "2px solid transparent",
                  outlineOffset: "2px",
                }}
              />
            ))}
          </div>
        </div>

        <div>
          <div className="flex items-center justify-between mb-2">
            <span className={LABEL}>{t("player.subtitleStyle.background")}</span>
            <span className="text-xs text-[var(--text-muted)]">{style.bgOpacity}%</span>
          </div>
          <div className="flex gap-2 mb-2">
            {SUBTITLE_BG_COLORS.map((c) => {
              const active = c.opacity === 0 ? style.bgOpacity === 0 : style.bgColor === c.value && style.bgOpacity > 0;
              return (
                <button
                  key={c.labelKey}
                  type="button"
                  title={t(c.labelKey)}
                  aria-label={t(c.labelKey)}
                  aria-pressed={active}
                  onClick={() => update({ bgColor: c.value, bgOpacity: c.opacity !== undefined ? c.opacity : style.bgOpacity || 75 })}
                  className="w-6 h-6 rounded border border-[var(--border)]"
                  style={{
                    backgroundColor: c.opacity === 0 ? "transparent" : c.value,
                    outline: active ? "2px solid var(--accent)" : "2px solid transparent",
                    outlineOffset: "2px",
                  }}
                />
              );
            })}
          </div>
          <input
            type="range"
            min={0}
            max={100}
            step={5}
            value={style.bgOpacity}
            onChange={(e) => update({ bgOpacity: Number(e.target.value) })}
            className="w-full accent-[var(--accent)]"
            aria-label={t("player.subtitleStyle.background")}
          />
        </div>

        <div className="flex flex-col gap-3">
          <div>
            <span className={`${LABEL} block mb-2`}>{t("player.subtitleStyle.position")}</span>
            <div className="flex gap-2">
              {(["bottom", "top"] as const).map((pos) => (
                <button
                  key={pos}
                  type="button"
                  aria-pressed={style.position === pos}
                  onClick={() => update({ position: pos })}
                  className={`flex-1 text-xs py-1.5 rounded border border-[var(--border)] ${style.position === pos ? "bg-[var(--accent)] text-[var(--filter-active-text)]" : "bg-[var(--surface-raised)] text-[var(--text)]"}`}
                >
                  {t(`player.subtitleStyle.${pos}`)}
                </button>
              ))}
            </div>
          </div>
          <label className="flex items-center gap-2 text-sm text-[var(--text)]">
            <input
              type="checkbox"
              checked={style.shadow}
              onChange={(e) => update({ shadow: e.target.checked })}
              className="accent-[var(--accent)]"
            />
            {t("player.subtitleStyle.textShadow")}
          </label>
        </div>
      </fieldset>
    </div>
  );
}
