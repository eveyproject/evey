import { type InputHTMLAttributes, useState } from "react";
import { Eye, EyeOff } from "lucide-react";
import { useTranslation } from "react-i18next";

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
}

export function Input({ label, error, className = "", id, type, ...props }: InputProps) {
  const { t } = useTranslation();
  const [revealed, setRevealed] = useState(false);
  const isPassword = type === "password";
  const inputId = id ?? label?.toLowerCase().replace(/\s+/g, "-");
  const errorId = error && inputId ? `${inputId}-error` : undefined;
  // Revealing a password just swaps the native type to text; everything else
  // (autocomplete, value, handlers) flows through ...props untouched.
  const effectiveType = isPassword && revealed ? "text" : type;

  const inputEl = (
    <input
      id={inputId}
      type={effectiveType}
      aria-invalid={error ? true : undefined}
      aria-describedby={errorId}
      {...props}
      className={`px-3 py-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent transition-shadow duration-200 ${error ? "border-[var(--color-danger)]" : ""} ${isPassword ? "w-full pr-10" : ""} ${className}`}
    />
  );

  return (
    <div className="flex flex-col gap-1">
      {label && (
        <label htmlFor={inputId} className="text-sm font-medium text-[var(--text-muted)]">
          {label}
        </label>
      )}
      {isPassword ? (
        <div className="relative">
          {inputEl}
          <button
            type="button"
            onClick={() => setRevealed((v) => !v)}
            aria-label={revealed ? t("auth.password.hide") : t("auth.password.show")}
            aria-pressed={revealed}
            title={revealed ? t("auth.password.hide") : t("auth.password.show")}
            className="absolute right-0 top-0 h-full px-3 flex items-center text-[var(--text-muted)] hover:text-[var(--text)] focus:outline-none focus:text-[var(--text)]"
          >
            {revealed ? <EyeOff size={16} /> : <Eye size={16} />}
          </button>
        </div>
      ) : (
        inputEl
      )}
      {error && <span id={errorId} className="text-xs text-[var(--color-danger)]" role="alert">{error}</span>}
    </div>
  );
}
