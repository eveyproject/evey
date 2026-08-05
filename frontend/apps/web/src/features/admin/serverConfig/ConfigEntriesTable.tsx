import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { OPERATOR_FLAGS, findOperatorFlag } from "@/features/admin/operatorFlags";
import { useSetConfigEntry, useDeleteConfigEntry, type ConfigEntry } from "./hooks";

export function ConfigRow({ entry }: { entry: ConfigEntry }) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(entry.value ?? "");
  const setEntry = useSetConfigEntry();
  const deleteEntry = useDeleteConfigEntry();

  function handleSave() {
    setEntry.mutate({ key: entry.key ?? "", value });
    setEditing(false);
  }

  return (
    <tr className="border-b border-[var(--border)]">
      <td data-label={t("admin.serverConfig.tableHeaders.key")} className="px-4 py-3 font-mono text-sm text-[var(--text)]">
        {entry.key}
      </td>
      <td data-label={t("admin.serverConfig.tableHeaders.value")} className="px-4 py-3 text-sm text-[var(--text)]">
        {editing ? (
          <input
            aria-label={`${entry.key} ${t("admin.serverConfig.tableHeaders.value")}`}
            className="w-full px-2 py-1 rounded text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleSave();
              if (e.key === "Escape") { setValue(entry.value ?? ""); setEditing(false); }
            }}
            autoFocus
          />
        ) : (
          <span className="font-mono text-xs break-all">{entry.value}</span>
        )}
      </td>
      <td data-label={t("admin.serverConfig.tableHeaders.updated")} className="px-4 py-3 text-xs whitespace-nowrap text-[var(--text-muted)]">
        {entry.updatedAt ? new Date(entry.updatedAt).toLocaleString() : ""}
      </td>
      <td data-label={t("admin.serverConfig.tableHeaders.actions")} className="px-4 py-3">
        <div className="flex gap-2">
          {editing ? (
            <>
              <Button variant="secondary" onClick={handleSave} loading={setEntry.isPending} className="text-xs">{t("common.save")}</Button>
              <Button variant="ghost" onClick={() => { setValue(entry.value ?? ""); setEditing(false); }} className="text-xs">{t("common.cancel")}</Button>
            </>
          ) : (
            <>
              <Button variant="secondary" onClick={() => setEditing(true)} className="text-xs">{t("common.edit")}</Button>
              <Button variant="danger" onClick={() => { const k = entry.key ?? ""; if (window.confirm(t("admin.serverConfig.deleteEntryConfirm", { key: k }))) deleteEntry.mutate(k); }} loading={deleteEntry.isPending} className="text-xs">{t("common.delete")}</Button>
            </>
          )}
        </div>
      </td>
    </tr>
  );
}

export function AddEntryRow() {
  const { t } = useTranslation();
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const setEntry = useSetConfigEntry();

  function handleAdd() {
    if (!key.trim()) return;
    setEntry.mutate({ key: key.trim(), value }, {
      onSuccess: () => { setKey(""); setValue(""); },
    });
  }

  // Suggest the boot-time operator flags. They were always settable here, but
  // only if you already knew the exact key to type - and nothing listed them.
  const known = findOperatorFlag(key);

  return (
    <tr className="border-b border-[var(--border)] bg-[var(--surface)]">
      <td data-label={t("admin.serverConfig.tableHeaders.key")} className="px-4 py-3">
        <input
          placeholder="key"
          aria-label={t("admin.serverConfig.tableHeaders.key")}
          list="operator-flag-keys"
          className="w-full px-2 py-1 rounded text-sm font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
        <datalist id="operator-flag-keys">
          {OPERATOR_FLAGS.map((f) => (
            <option key={f.key} value={f.key} />
          ))}
        </datalist>
        {known && (
          <p className="text-xs text-[var(--text-muted)] mt-1" data-testid="operator-flag-hint">
            {known.valueHint} · {known.envVar} · {t("admin.serverConfig.appliedOnRestart", "applied on restart")}
          </p>
        )}
      </td>
      <td data-label={t("admin.serverConfig.tableHeaders.value")} className="px-4 py-3">
        <input
          placeholder="value"
          aria-label={t("admin.serverConfig.tableHeaders.value")}
          className="w-full px-2 py-1 rounded text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") handleAdd(); }}
        />
      </td>
      <td className="hidden md:table-cell" />
      <td data-label={t("admin.serverConfig.tableHeaders.actions")} className="px-4 py-3">
        <Button onClick={handleAdd} loading={setEntry.isPending} className="text-xs" disabled={!key.trim()}>{t("common.add")}</Button>
      </td>
    </tr>
  );
}
