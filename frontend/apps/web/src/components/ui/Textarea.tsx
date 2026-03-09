import { type TextareaHTMLAttributes } from "react";

interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
  error?: string;
}

export function Textarea({ label, error, className = "", id, rows = 4, ...props }: TextareaProps) {
  const textareaId = id ?? label?.toLowerCase().replace(/\s+/g, "-");
  return (
    <div className="flex flex-col gap-1">
      {label && (
        <label htmlFor={textareaId} className="text-sm font-medium text-[var(--text-muted)]">
          {label}
        </label>
      )}
      <textarea
        id={textareaId}
        rows={rows}
        {...props}
        className={`px-3 py-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] text-sm placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent resize-y ${error ? "border-red-500" : ""} ${className}`}
      />
      {error && <span className="text-xs text-red-400">{error}</span>}
    </div>
  );
}
