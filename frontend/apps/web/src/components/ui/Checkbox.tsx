import { useId, type InputHTMLAttributes } from "react";

interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "type" | "checked" | "onChange"> {
  label?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
}

export function Checkbox({ label, checked, onChange, disabled, className = "", id, ...props }: CheckboxProps) {
  const autoId = useId();
  const inputId = id ?? autoId;
  return (
    <label
      htmlFor={inputId}
      className={`inline-flex items-center gap-2 cursor-pointer select-none text-sm text-[var(--text)] ${disabled ? "opacity-50 cursor-not-allowed" : ""} ${className}`}
    >
      <input
        id={inputId}
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
        className="w-4 h-4 rounded-[var(--radius-sm)] border border-[var(--border)] accent-[var(--accent)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 cursor-pointer disabled:cursor-not-allowed"
        {...props}
      />
      {label && <span>{label}</span>}
    </label>
  );
}
