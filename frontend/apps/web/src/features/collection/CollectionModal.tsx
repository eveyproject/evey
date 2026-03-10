import { useState, useEffect } from "react";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import type { Collection, CollectionRule } from "./api";

interface CollectionModalProps {
  open: boolean;
  onClose: () => void;
  onSubmit: (data: {
    name: string;
    overview?: string;
    type: string;
    rules: CollectionRule[];
    sortBy?: string;
    sortOrder?: string;
  }) => void;
  initial?: Collection | null;
}

const FIELDS = [
  { value: "genre", label: "Genre" },
  { value: "year", label: "Year" },
  { value: "type", label: "Type" },
  { value: "rating", label: "Rating" },
  { value: "resolution", label: "Resolution" },
  { value: "added_date", label: "Added (days ago)" },
];

const OPERATORS: Record<string, { value: string; label: string }[]> = {
  genre: [
    { value: "contains", label: "contains" },
    { value: "eq", label: "equals" },
  ],
  year: [
    { value: "eq", label: "=" },
    { value: "gt", label: ">" },
    { value: "lt", label: "<" },
  ],
  type: [
    { value: "eq", label: "equals" },
    { value: "neq", label: "not equals" },
  ],
  rating: [
    { value: "gt", label: ">" },
    { value: "lt", label: "<" },
    { value: "eq", label: "=" },
  ],
  resolution: [
    { value: "eq", label: "equals" },
  ],
  added_date: [
    { value: "lt", label: "within last" },
  ],
};

function emptyRule(): CollectionRule {
  return { field: "genre", operator: "contains", value: "" };
}

export function CollectionModal({ open, onClose, onSubmit, initial }: CollectionModalProps) {
  const [name, setName] = useState("");
  const [overview, setOverview] = useState("");
  const [type, setType] = useState<"smart" | "manual">("smart");
  const [rules, setRules] = useState<CollectionRule[]>([emptyRule()]);
  const [sortBy, setSortBy] = useState("sort_title");
  const [sortOrder, setSortOrder] = useState("asc");

  useEffect(() => {
    if (initial) {
      setName(initial.name);
      setOverview(initial.overview ?? "");
      setType(initial.type);
      setRules(initial.rules.length > 0 ? initial.rules : [emptyRule()]);
      setSortBy(initial.sortBy || "sort_title");
      setSortOrder(initial.sortOrder || "asc");
    } else {
      setName("");
      setOverview("");
      setType("smart");
      setRules([emptyRule()]);
      setSortBy("sort_title");
      setSortOrder("asc");
    }
  }, [initial, open]);

  function updateRule(idx: number, patch: Partial<CollectionRule>) {
    setRules((prev) => prev.map((r, i) => (i === idx ? { ...r, ...patch } : r)));
  }

  function removeRule(idx: number) {
    setRules((prev) => prev.filter((_, i) => i !== idx));
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    onSubmit({
      name,
      overview: overview || undefined,
      type,
      rules: type === "smart" ? rules.filter((r) => r.value) : [],
      sortBy,
      sortOrder,
    });
    onClose();
  }

  return (
    <Modal open={open} onClose={onClose} title={initial ? "Edit Collection" : "New Collection"}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <Input
          label="Name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          required
        />
        <Input
          label="Description"
          value={overview}
          onChange={(e) => setOverview(e.target.value)}
        />
        <Select
          label="Type"
          value={type}
          onChange={(v) => setType(v as "smart" | "manual")}
          options={[
            { value: "smart", label: "Smart (auto-populate)" },
            { value: "manual", label: "Manual" },
          ]}
        />

        {type === "smart" && (
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium" style={{ color: "var(--text)" }}>
                Rules
              </span>
              <button
                type="button"
                onClick={() => setRules((prev) => [...prev, emptyRule()])}
                className="text-xs px-2 py-1 rounded"
                style={{ backgroundColor: "var(--surface-raised)", color: "var(--accent)" }}
              >
                + Add rule
              </button>
            </div>
            {rules.map((rule, idx) => (
              <div key={idx} className="flex items-center gap-2">
                <select
                  value={rule.field}
                  onChange={(e) =>
                    updateRule(idx, {
                      field: e.target.value,
                      operator: OPERATORS[e.target.value]?.[0]?.value ?? "eq",
                    })
                  }
                  className="text-sm px-2 py-1.5 rounded"
                  style={{
                    backgroundColor: "var(--surface-raised)",
                    color: "var(--text)",
                    border: "1px solid var(--border)",
                  }}
                >
                  {FIELDS.map((f) => (
                    <option key={f.value} value={f.value}>
                      {f.label}
                    </option>
                  ))}
                </select>
                <select
                  value={rule.operator}
                  onChange={(e) => updateRule(idx, { operator: e.target.value })}
                  className="text-sm px-2 py-1.5 rounded"
                  style={{
                    backgroundColor: "var(--surface-raised)",
                    color: "var(--text)",
                    border: "1px solid var(--border)",
                  }}
                >
                  {(OPERATORS[rule.field] ?? []).map((op) => (
                    <option key={op.value} value={op.value}>
                      {op.label}
                    </option>
                  ))}
                </select>
                {rule.field === "resolution" ? (
                  <select
                    value={rule.value}
                    onChange={(e) => updateRule(idx, { value: e.target.value })}
                    className="text-sm px-2 py-1.5 rounded flex-1"
                    style={{
                      backgroundColor: "var(--surface-raised)",
                      color: "var(--text)",
                      border: "1px solid var(--border)",
                    }}
                  >
                    <option value="">Select...</option>
                    <option value="4k">4K</option>
                    <option value="1080p">1080p</option>
                    <option value="720p">720p</option>
                    <option value="480p">480p</option>
                  </select>
                ) : rule.field === "type" ? (
                  <select
                    value={rule.value}
                    onChange={(e) => updateRule(idx, { value: e.target.value })}
                    className="text-sm px-2 py-1.5 rounded flex-1"
                    style={{
                      backgroundColor: "var(--surface-raised)",
                      color: "var(--text)",
                      border: "1px solid var(--border)",
                    }}
                  >
                    <option value="">Select...</option>
                    <option value="movie">Movie</option>
                    <option value="series">Series</option>
                    <option value="episode">Episode</option>
                  </select>
                ) : (
                  <input
                    type="text"
                    value={rule.value}
                    onChange={(e) => updateRule(idx, { value: e.target.value })}
                    placeholder={
                      rule.field === "genre"
                        ? "e.g. Action"
                        : rule.field === "year"
                          ? "e.g. 2020"
                          : rule.field === "rating"
                            ? "e.g. 7.0"
                            : rule.field === "added_date"
                              ? "e.g. 30"
                              : ""
                    }
                    className="text-sm px-2 py-1.5 rounded flex-1"
                    style={{
                      backgroundColor: "var(--surface-raised)",
                      color: "var(--text)",
                      border: "1px solid var(--border)",
                    }}
                  />
                )}
                {rules.length > 1 && (
                  <button
                    type="button"
                    onClick={() => removeRule(idx)}
                    className="text-sm px-1.5 py-1 rounded"
                    style={{ color: "var(--text-muted)" }}
                  >
                    x
                  </button>
                )}
              </div>
            ))}
          </div>
        )}

        <div className="flex gap-3">
          <Select
            label="Sort by"
            value={sortBy}
            onChange={(v) => setSortBy(v)}
            options={[
              { value: "sort_title", label: "Title" },
              { value: "added_at", label: "Date added" },
              { value: "year", label: "Year" },
              { value: "community_rating", label: "Rating" },
            ]}
          />
          <Select
            label="Order"
            value={sortOrder}
            onChange={(v) => setSortOrder(v)}
            options={[
              { value: "asc", label: "Ascending" },
              { value: "desc", label: "Descending" },
            ]}
          />
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={!name.trim()}>
            {initial ? "Save" : "Create"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
