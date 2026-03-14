import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api/src/fetcher";
import { toast, useLocaleStore } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";

export const Route = createLazyFileRoute("/_auth/admin/server-config")({
  component: ServerConfigPage,
});

interface ConfigEntry {
  key: string;
  value: string;
  updatedAt: string;
}

const QUERY_KEY = ["/api/v1/admin/server-config"];

function useServerConfig() {
  return useQuery<ConfigEntry[]>({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const data = await eveyFetch<{ entries: ConfigEntry[] }>({ url: "/api/v1/admin/server-config", method: "GET" });
      return data.entries ?? [];
    },
  });
}

function useSetConfigEntry() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      eveyFetch<void>({ url: `/api/v1/admin/server-config/${encodeURIComponent(key)}`, method: "PUT", data: { value } }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      toast.success("Config updated");
    },
    onError: () => toast.error("Failed to update config"),
  });
}

function useDeleteConfigEntry() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (key: string) =>
      eveyFetch<void>({ url: `/api/v1/admin/server-config/${encodeURIComponent(key)}`, method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      toast.success("Config entry deleted");
    },
    onError: () => toast.error("Failed to delete entry"),
  });
}

function ConfigRow({ entry }: { entry: ConfigEntry }) {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(entry.value);
  const setEntry = useSetConfigEntry();
  const deleteEntry = useDeleteConfigEntry();

  function handleSave() {
    setEntry.mutate({ key: entry.key, value });
    setEditing(false);
  }

  return (
    <tr style={{ borderBottom: "1px solid var(--border)" }}>
      <td className="px-4 py-3 font-mono text-sm" style={{ color: "var(--text)" }}>
        {entry.key}
      </td>
      <td className="px-4 py-3 text-sm" style={{ color: "var(--text)" }}>
        {editing ? (
          <input
            className="w-full px-2 py-1 rounded text-sm"
            style={{
              backgroundColor: "var(--surface-raised)",
              border: "1px solid var(--border)",
              color: "var(--text)",
            }}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleSave();
              if (e.key === "Escape") { setValue(entry.value); setEditing(false); }
            }}
            autoFocus
          />
        ) : (
          <span className="font-mono text-xs break-all">{entry.value}</span>
        )}
      </td>
      <td className="px-4 py-3 text-xs whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
        {new Date(entry.updatedAt).toLocaleString()}
      </td>
      <td className="px-4 py-3">
        <div className="flex gap-2">
          {editing ? (
            <>
              <Button variant="secondary" onClick={handleSave} loading={setEntry.isPending} className="text-xs">Save</Button>
              <Button variant="ghost" onClick={() => { setValue(entry.value); setEditing(false); }} className="text-xs">Cancel</Button>
            </>
          ) : (
            <>
              <Button variant="secondary" onClick={() => setEditing(true)} className="text-xs">Edit</Button>
              <Button variant="danger" onClick={() => deleteEntry.mutate(entry.key)} loading={deleteEntry.isPending} className="text-xs">Delete</Button>
            </>
          )}
        </div>
      </td>
    </tr>
  );
}

const LOCALE_OPTIONS = [
  { label: "English (US)", value: "en-US" },
  { label: "Français (FR)", value: "fr-FR" },
  { label: "Deutsch (DE)", value: "de-DE" },
  { label: "Español (ES)", value: "es-ES" },
  { label: "Italiano (IT)", value: "it-IT" },
  { label: "Português (BR)", value: "pt-BR" },
  { label: "日本語 (JP)", value: "ja-JP" },
  { label: "한국어 (KR)", value: "ko-KR" },
  { label: "中文 (CN)", value: "zh-CN" },
];

const METADATA_LANG_KEY = "metadata.language";

function LocaleSettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const setEntry = useSetConfigEntry();
  const setLocale = useLocaleStore((s) => s.setLocale);

  const current = entries?.find((e) => e.key === METADATA_LANG_KEY)?.value ?? "";

  function handleChange(value: string) {
    setEntry.mutate(
      { key: METADATA_LANG_KEY, value },
      {
        onSuccess: () => {
          setLocale(value);
          toast.success("Language updated — restart server to apply to metadata fetching");
        },
      }
    );
  }

  return (
    <div
      className="rounded-[var(--radius-lg)] p-5"
      style={{ border: "1px solid var(--border)", backgroundColor: "var(--surface)" }}
    >
      <h2 className="text-base font-semibold mb-1" style={{ color: "var(--text)" }}>
        Language &amp; Region
      </h2>
      <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
        Metadata language used when fetching from TMDB and other providers. Takes effect after server restart.
      </p>
      <div className="flex items-center gap-3">
        <label className="text-sm font-medium" style={{ color: "var(--text)" }} htmlFor="metadata-lang">
          Metadata language
        </label>
        <select
          id="metadata-lang"
          value={current}
          onChange={(e) => handleChange(e.target.value)}
          className="px-3 py-1.5 rounded text-sm"
          style={{
            backgroundColor: "var(--surface-raised)",
            border: "1px solid var(--border)",
            color: "var(--text)",
          }}
        >
          <option value="">— env default —</option>
          {LOCALE_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
        {setEntry.isPending && <Spinner size="sm" />}
      </div>
    </div>
  );
}

function AddEntryRow() {
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const setEntry = useSetConfigEntry();

  function handleAdd() {
    if (!key.trim()) return;
    setEntry.mutate({ key: key.trim(), value }, {
      onSuccess: () => { setKey(""); setValue(""); },
    });
  }

  return (
    <tr style={{ borderBottom: "1px solid var(--border)", backgroundColor: "var(--surface)" }}>
      <td className="px-4 py-3">
        <input
          placeholder="key"
          className="w-full px-2 py-1 rounded text-sm font-mono"
          style={{ backgroundColor: "var(--surface-raised)", border: "1px solid var(--border)", color: "var(--text)" }}
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
      </td>
      <td className="px-4 py-3">
        <input
          placeholder="value"
          className="w-full px-2 py-1 rounded text-sm"
          style={{ backgroundColor: "var(--surface-raised)", border: "1px solid var(--border)", color: "var(--text)" }}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") handleAdd(); }}
        />
      </td>
      <td />
      <td className="px-4 py-3">
        <Button onClick={handleAdd} loading={setEntry.isPending} className="text-xs" disabled={!key.trim()}>Add</Button>
      </td>
    </tr>
  );
}

function ServerConfigPage() {
  const { data: entries, isLoading } = useServerConfig();

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>Server Config</h1>
        <p className="text-sm mt-1" style={{ color: "var(--text-muted)" }}>
          Key-value configuration entries persisted in the database.
        </p>
      </div>

      <LocaleSettingsCard entries={entries} />

      {isLoading ? (
        <div className="flex justify-center py-16"><Spinner size="lg" /></div>
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)]" style={{ border: "1px solid var(--border)" }}>
          <table className="w-full text-sm">
            <thead>
              <tr style={{ backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
                {["Key", "Value", "Updated", "Actions"].map((h) => (
                  <th key={h} className="px-4 py-2 text-left font-medium" style={{ color: "var(--text-muted)" }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {entries && entries.length === 0 ? (
                <tr>
                  <td colSpan={4}>
                    <EmptyState icon="library" title="No config entries" description="Add key-value entries below." />
                  </td>
                </tr>
              ) : (
                entries?.map((e) => <ConfigRow key={e.key} entry={e} />)
              )}
              <AddEntryRow />
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
