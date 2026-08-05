import { useState, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const PARSER_CONFIG_KEY = "parser.config";

interface ParserConfigShape {
  extra_qualities?: string[];
  extra_edition_suffixes?: string[];
}

/**
 * Lets the admin extend the filename parser's quality / edition token
 * lists without redeploying. Baseline tokens stay hardcoded in the
 * filesystem package - this UI only manages the EXTENSIONS, persisted as
 * a single JSON blob under the parser.config server_config key. The
 * backend subscriber re-applies them in-process on every save.
 *
 * Quality tokens must be single-word (the parser splits filenames on
 * "." "_" "-" SPACE); edition suffixes can span multiple words because
 * they're stripped via regex before tokenisation.
 */
export function ParserTokensCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const raw = entries?.find((e) => e.key === PARSER_CONFIG_KEY)?.value ?? "";
  const initial: ParserConfigShape = useMemo(() => {
    if (!raw) return {};
    try {
      return JSON.parse(raw) as ParserConfigShape;
    } catch {
      return {};
    }
  }, [raw]);

  const initialQualities = (initial.extra_qualities ?? []).join(", ");
  const initialEditions = (initial.extra_edition_suffixes ?? []).join(", ");

  const [qualities, setQualities] = useState(initialQualities);
  const [editions, setEditions] = useState(initialEditions);
  const [dirty, setDirty] = useState(false);

  // Re-sync when the persisted blob changes externally.
  useEffect(() => {
    setQualities(initialQualities);
    setEditions(initialEditions);
    setDirty(false);
  }, [initialQualities, initialEditions]);

  function handleSave() {
    const parse = (s: string) =>
      s.split(",").map((x) => x.trim()).filter((x) => x.length > 0);
    const payload: ParserConfigShape = {
      extra_qualities: parse(qualities),
      extra_edition_suffixes: parse(editions),
    };
    setEntry.mutate({ key: PARSER_CONFIG_KEY, value: JSON.stringify(payload) });
    setDirty(false);
  }

  return (
    <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium text-[var(--text)]">
            {t("admin.serverConfig.parserTokensQualities", "Extra quality tokens")}
          </label>
          <textarea
            data-testid="parser-tokens-qualities"
            value={qualities}
            onChange={(e) => { setQualities(e.target.value); setDirty(true); }}
            rows={2}
            className="w-full px-2 py-1 rounded text-sm font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
            placeholder="hdrip, hdr, web-dl"
          />
          <p className="text-xs text-[var(--text-muted)]">
            {t(
              "admin.serverConfig.parserTokensQualitiesHint",
              "Comma-separated, single-word tokens (lowercased). The parser splits filenames on '.', '_', '-' and space.",
            )}
          </p>
        </div>
        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium text-[var(--text)]">
            {t("admin.serverConfig.parserTokensEditions", "Extra edition suffixes")}
          </label>
          <textarea
            data-testid="parser-tokens-editions"
            value={editions}
            onChange={(e) => { setEditions(e.target.value); setDirty(true); }}
            rows={2}
            className="w-full px-2 py-1 rounded text-sm font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
            placeholder="distributor cut, anniversary edition, imax"
          />
          <p className="text-xs text-[var(--text-muted)]">
            {t(
              "admin.serverConfig.parserTokensEditionsHint",
              "Comma-separated, can span multiple words (regex pre-tokenisation). Stripped from the title before TMDB lookup.",
            )}
          </p>
        </div>
        <div className="flex justify-end pt-1">
          <Button
            onClick={handleSave}
            loading={setEntry.isPending}
            disabled={!dirty}
            data-testid="parser-tokens-save-btn"
          >
            {t("common.save")}
          </Button>
        </div>
    </div>
  );
}
