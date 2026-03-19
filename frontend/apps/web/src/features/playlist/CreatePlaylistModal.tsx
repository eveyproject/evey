import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Textarea } from "@/components/ui/Textarea";
import { Select } from "@/components/ui/Select";
import { toast } from "@evey/store";
import { useCreatePlaylist, type PlaylistRule } from "./api";

interface Props {
  open: boolean;
  onClose: () => void;
}

const RULE_FIELDS = [
  { value: "genre", label: "Genre" },
  { value: "type", label: "Type" },
  { value: "year", label: "Year" },
  { value: "library_id", label: "Library" },
  { value: "rating_gte", label: "Rating >=" },
];

const RULE_OPERATORS = [
  { value: "eq", label: "equals" },
  { value: "gt", label: ">" },
  { value: "gte", label: ">=" },
  { value: "lt", label: "<" },
  { value: "lte", label: "<=" },
];

export function CreatePlaylistModal({ open, onClose }: Props) {
  const { t } = useTranslation("media");
  const { t: tc } = useTranslation("common");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [type, setType] = useState<"manual" | "smart">("manual");
  const [rules, setRules] = useState<PlaylistRule[]>([]);
  const create = useCreatePlaylist();

  function addRule() {
    setRules((prev) => [...prev, { field: "genre", operator: "eq", value: "" }]);
  }

  function updateRule(index: number, field: keyof PlaylistRule, value: string) {
    setRules((prev) => prev.map((r, i) => (i === index ? { ...r, [field]: value } : r)));
  }

  function removeRule(index: number) {
    setRules((prev) => prev.filter((_, i) => i !== index));
  }

  function reset() {
    setName("");
    setDescription("");
    setType("manual");
    setRules([]);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    try {
      await create.mutateAsync({
        name: name.trim(),
        description: description.trim(),
        type,
        rules: type === "smart" ? rules.filter((r) => r.value.trim()) : undefined,
      });
      toast.success(t("playlist.createSuccess", { name }));
      reset();
      onClose();
    } catch {
      toast.error(t("playlist.createError"));
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("playlist.createTitle")}>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <Input label={t("playlist.name")} value={name} onChange={(e) => setName(e.target.value)} required />
        <Textarea label={t("playlist.description")} value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
        <Select
          label={t("playlist.type")}
          value={type}
          onChange={(v) => setType(v as "manual" | "smart")}
          options={[
            { value: "manual", label: t("playlist.manual") },
            { value: "smart", label: t("playlist.smart") },
          ]}
        />

        {type === "smart" && (
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium" style={{ color: "var(--text)" }}>{t("playlist.rules")}</span>
              <Button type="button" variant="secondary" onClick={addRule}>
                {t("playlist.addRule")}
              </Button>
            </div>
            {rules.map((rule, i) => (
              <div key={i} className="flex gap-2 items-end">
                <Select
                  label={i === 0 ? t("playlist.field") : undefined}
                  value={rule.field}
                  onChange={(v) => updateRule(i, "field", v)}
                  options={RULE_FIELDS}
                />
                <Select
                  label={i === 0 ? t("playlist.op") : undefined}
                  value={rule.operator}
                  onChange={(v) => updateRule(i, "operator", v)}
                  options={RULE_OPERATORS}
                />
                <Input
                  label={i === 0 ? t("playlist.value") : undefined}
                  value={rule.value}
                  onChange={(e) => updateRule(i, "value", e.target.value)}
                  placeholder="e.g. Action"
                />
                <button
                  type="button"
                  onClick={() => removeRule(i)}
                  className="p-2 rounded hover:bg-[var(--surface-raised)] transition-colors"
                  style={{ color: "var(--text-muted)" }}
                >
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            ))}
          </div>
        )}

        <div className="flex gap-2 pt-2">
          <Button type="submit" loading={create.isPending}>{tc("create")}</Button>
          <Button type="button" variant="secondary" onClick={onClose}>{tc("cancel")}</Button>
        </div>
      </form>
    </Modal>
  );
}
