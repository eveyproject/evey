import { useState } from "react";
import { X } from "lucide-react";
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
  const { t } = useTranslation();
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
      toast.success(t("media.playlist.createSuccess", { name }));
      reset();
      onClose();
    } catch {
      toast.error(t("media.playlist.createError"));
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("media.playlist.createTitle")}>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <Input label={t("media.playlist.name")} value={name} onChange={(e) => setName(e.target.value)} required />
        <Textarea label={t("media.playlist.description")} value={description} onChange={(e) => setDescription(e.target.value)} rows={2} />
        <Select
          label={t("media.playlist.type")}
          value={type}
          onChange={(v) => setType(v as "manual" | "smart")}
          options={[
            { value: "manual", label: t("media.playlist.manual") },
            { value: "smart", label: t("media.playlist.smart") },
          ]}
        />

        {type === "smart" && (
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-[var(--text)]">{t("media.playlist.rules")}</span>
              <Button type="button" variant="secondary" onClick={addRule}>
                {t("media.playlist.addRule")}
              </Button>
            </div>
            {rules.map((rule, i) => (
              <div key={i} className="flex gap-2 items-end">
                <Select
                  label={i === 0 ? t("media.playlist.field") : undefined}
                  value={rule.field}
                  onChange={(v) => updateRule(i, "field", v)}
                  options={RULE_FIELDS}
                />
                <Select
                  label={i === 0 ? t("media.playlist.op") : undefined}
                  value={rule.operator}
                  onChange={(v) => updateRule(i, "operator", v)}
                  options={RULE_OPERATORS}
                />
                <Input
                  label={i === 0 ? t("media.playlist.value") : undefined}
                  value={rule.value}
                  onChange={(e) => updateRule(i, "value", e.target.value)}
                  placeholder="e.g. Action"
                />
                <button
                  type="button"
                  onClick={() => removeRule(i)}
                  className="p-2 rounded hover:bg-[var(--surface-raised)] transition-colors-fast text-[var(--text-muted)]"
                  aria-label={t("media.playlist.removeRule", "Remove rule")}
                >
                  <X size={16} />
                </button>
              </div>
            ))}
          </div>
        )}

        <div className="flex gap-2 pt-2">
          <Button type="submit" loading={create.isPending}>{t("common.create")}</Button>
          <Button type="button" variant="secondary" onClick={onClose}>{t("common.cancel")}</Button>
        </div>
      </form>
    </Modal>
  );
}
