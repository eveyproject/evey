import { useState, useEffect, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { SettingRow } from "@/components/ui/SettingRow";
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

  const textareaClass =
    "w-full px-3 py-2 rounded-[var(--radius-md)] text-sm font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]";
  return (
    <div className="flex flex-col divide-y divide-[var(--border)]">
      <SettingRow
        label={t("admin.serverConfig.parserTokensQualities", "Extra quality tokens")}
        htmlFor="parser-tokens-qualities"
        info={t("admin.settingsHelp.parserQualities")}
        hint={t("admin.settingsLayout.commaSeparated")}
        kind="wide"
      >
        <textarea
          id="parser-tokens-qualities"
          data-testid="parser-tokens-qualities"
          value={qualities}
          onChange={(e) => { setQualities(e.target.value); setDirty(true); }}
          rows={2}
          className={textareaClass}
          placeholder="hdrip, hdr, web-dl"
        />
      </SettingRow>
      <SettingRow
        label={t("admin.serverConfig.parserTokensEditions", "Extra edition suffixes")}
        htmlFor="parser-tokens-editions"
        info={t("admin.settingsHelp.parserEditions")}
        hint={t("admin.settingsLayout.commaSeparated")}
        kind="wide"
      >
        <textarea
          id="parser-tokens-editions"
          data-testid="parser-tokens-editions"
          value={editions}
          onChange={(e) => { setEditions(e.target.value); setDirty(true); }}
          rows={2}
          className={textareaClass}
          placeholder="distributor cut, anniversary edition, imax"
        />
      </SettingRow>
      <div className="flex justify-end pt-3">
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
