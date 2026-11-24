import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Check, ListMusic, Sparkles } from "lucide-react";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ICON } from "@/constants/iconSizes";
import { useCreatePlaylist, useGeneratePlaylist, useMixOptions } from "@/features/playlist/api";

// A new music playlist, in words anyone reads: either empty, to fill by hand,
// or a mix the server draws from the styles ticked here, leaning on what the
// profile listens to most. The rule editor of the video playlists (field,
// operator, value) never applied to music tracks anyway.

type Mode = "manual" | "mix";

const SIZES = [25, 50, 100] as const;

// How many styles show before "more": a big library has dozens.
const GENRES_SHOWN = 18;

export function MusicPlaylistModal({
  open,
  onClose,
  libraryID,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  libraryID?: string;
  onCreated: (id: string) => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  // Until the name is typed in, a mix shows the one its styles suggest, as
  // real text the person can change, not a grey placeholder.
  const [nameTouched, setNameTouched] = useState(false);
  const [mode, setMode] = useState<Mode>("manual");
  const [genres, setGenres] = useState<string[]>([]);
  const [size, setSize] = useState<number>(50);
  const [showAll, setShowAll] = useState(false);
  const create = useCreatePlaylist();
  const generate = useGeneratePlaylist();
  const { data: options, isLoading: optionsLoading } = useMixOptions(libraryID, open && mode === "mix");
  const busy = create.isPending || generate.isPending;

  const allGenres = options?.genres ?? [];
  const shownGenres = showAll ? allGenres : allGenres.slice(0, GENRES_SHOWN);
  // A ticked style stays visible even when it sits past the first ones.
  const extraTicked = showAll ? [] : allGenres.slice(GENRES_SHOWN).filter((g) => genres.includes(g.name));

  // The name the mix gets when none is typed: its styles, or the server's own.
  const suggested = mode === "mix" && genres.length > 0 ? `Mix ${genres.slice(0, 3).join(", ")}` : "";
  const shownName = nameTouched ? name : name || suggested;

  function reset() {
    setName("");
    setNameTouched(false);
    setMode("manual");
    setGenres([]);
    setSize(50);
    setShowAll(false);
  }

  function close() {
    reset();
    onClose();
  }

  function toggle(g: string) {
    setGenres((prev) => (prev.includes(g) ? prev.filter((x) => x !== g) : [...prev, g]));
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    const finalName = shownName.trim();
    try {
      if (mode === "manual") {
        if (!finalName) return;
        const created = await create.mutateAsync({ name: finalName, type: "manual" });
        toast.success(t("media.playlist.createSuccess", { name: finalName }));
        close();
        if (created?.id) onCreated(created.id);
        return;
      }
      if (!libraryID) return;
      const res = await generate.mutateAsync({ libraryId: libraryID, name: finalName || undefined, size, genres });
      toast.success(t("media.playlist.generated"));
      close();
      if (res?.id) onCreated(res.id);
    } catch (err) {
      const status = (err as { httpStatus?: number })?.httpStatus;
      if (status === 409) toast.error(t("media.music.mixNameTaken"));
      else if (status === 422) toast.error(t("media.music.mixNoTracks"));
      else toast.error(mode === "mix" ? t("media.playlist.generateFailed") : t("media.playlist.createError"));
    }
  }

  const choice = (value: Mode, icon: React.ReactNode, title: string, hint: string) => (
    <button
      type="button"
      role="radio"
      aria-checked={mode === value}
      onClick={() => setMode(value)}
      data-testid={`music-playlist-mode-${value}`}
      className={`flex-1 min-w-0 text-left p-3 rounded-[var(--radius-md)] border cursor-pointer transition-colors-fast ${
        mode === value
          ? "border-[var(--accent)] bg-[var(--surface-raised)]"
          : "border-[var(--border)] bg-transparent hover:bg-[var(--surface-raised)]"
      }`}
    >
      <span className="flex items-center gap-2 text-sm font-medium text-[var(--text)]">
        {icon}
        {title}
      </span>
      <span className="block mt-1 text-xs text-[var(--text-muted)]">{hint}</span>
    </button>
  );

  return (
    <Modal open={open} onClose={close} title={t("media.playlist.createTitle")} size="lg">
      <form onSubmit={submit} className="flex flex-col gap-4" data-testid="music-playlist-modal">
        <Input
          label={t("media.playlist.name")}
          value={shownName}
          onChange={(e) => { setName(e.target.value); setNameTouched(true); }}
          placeholder={mode === "mix" ? t("media.music.mixNamePlaceholder") : undefined}
          required={mode === "manual"}
        />

        <div className="flex flex-col gap-2">
          <span className="text-sm text-[var(--text-muted)]">{t("media.music.fillHow")}</span>
          <div role="radiogroup" className="flex flex-col sm:flex-row gap-2">
            {choice("manual", <ListMusic size={ICON.sm} aria-hidden="true" />, t("media.music.fillManual"), t("media.music.fillManualHint"))}
            {choice("mix", <Sparkles size={ICON.sm} aria-hidden="true" />, t("media.music.fillMix"), t("media.music.fillMixHint"))}
          </div>
        </div>

        {mode === "mix" && (
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-2">
              <span className="text-sm text-[var(--text)]">{t("media.music.mixStyles")}</span>
              {optionsLoading ? (
                <p className="text-xs text-[var(--text-muted)]">{t("media.music.loading")}</p>
              ) : allGenres.length === 0 ? (
                <p className="text-xs text-[var(--text-muted)]">{t("media.music.mixNoGenres")}</p>
              ) : (
                <div className="flex flex-wrap gap-1.5" data-testid="music-mix-genres">
                  {[...shownGenres, ...extraTicked].map((g) => {
                    const on = genres.includes(g.name);
                    return (
                      <button
                        key={g.name}
                        type="button"
                        aria-pressed={on}
                        onClick={() => toggle(g.name)}
                        className={`inline-flex items-center gap-1 px-2.5 py-1 rounded-full text-xs border cursor-pointer transition-colors-fast ${
                          on
                            ? "bg-[var(--accent)] text-[var(--filter-active-text)] border-[var(--accent)]"
                            : "bg-[var(--surface-raised)] text-[var(--text)] border-[var(--border)] hover:border-[var(--accent)]"
                        }`}
                      >
                        {on && <Check size={12} aria-hidden="true" />}
                        {g.name}
                        <span className={on ? "opacity-80" : "text-[var(--text-muted)]"}>{g.count}</span>
                      </button>
                    );
                  })}
                  {!showAll && allGenres.length > GENRES_SHOWN && (
                    <button
                      type="button"
                      onClick={() => setShowAll(true)}
                      className="px-2.5 py-1 rounded-full text-xs border border-dashed border-[var(--border)] text-[var(--text-muted)] bg-transparent cursor-pointer"
                    >
                      {t("media.music.mixMoreStyles", { count: allGenres.length - GENRES_SHOWN })}
                    </button>
                  )}
                </div>
              )}
              <p className="text-xs text-[var(--text-muted)]">
                {genres.length === 0 ? t("media.music.mixAllStyles") : t("media.music.mixSomeStyles", { count: genres.length })}
              </p>
            </div>

            <div className="flex flex-col gap-2">
              <span className="text-sm text-[var(--text)]">{t("media.music.mixLength")}</span>
              <div className="flex gap-1.5" role="radiogroup">
                {SIZES.map((n) => (
                  <button
                    key={n}
                    type="button"
                    role="radio"
                    aria-checked={size === n}
                    onClick={() => setSize(n)}
                    className={`px-3 py-1 rounded-[var(--radius-sm)] text-xs border cursor-pointer ${
                      size === n
                        ? "bg-[var(--accent)] text-[var(--filter-active-text)] border-[var(--accent)]"
                        : "bg-[var(--surface-raised)] text-[var(--text)] border-[var(--border)]"
                    }`}
                  >
                    {t("media.music.trackCount", { count: n })}
                  </button>
                ))}
              </div>
            </div>

            <p className="text-xs text-[var(--text-muted)] flex items-start gap-1.5">
              <Sparkles size={12} className="mt-0.5 shrink-0" aria-hidden="true" />
              {options?.hasHistory ? t("media.music.mixFromHistory") : t("media.music.mixNoHistory")}
            </p>
          </div>
        )}

        <div className="flex gap-2">
          <Button type="submit" variant="primary" loading={busy} disabled={busy || (mode === "manual" && !shownName.trim()) || (mode === "mix" && !libraryID)}>
            {mode === "mix" ? t("media.music.mixCreate") : t("media.playlist.createPlaylist")}
          </Button>
          <Button type="button" variant="secondary" onClick={close}>
            {t("common.cancel")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
