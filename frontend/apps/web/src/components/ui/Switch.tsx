interface SwitchProps {
  checked: boolean;
  onChange: () => void;
  label: string;
  disabled?: boolean;
  "data-testid"?: string;
}

export function Switch({ checked, onChange, label, disabled, "data-testid": testId }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      data-testid={testId}
      disabled={disabled}
      onClick={onChange}
      className="w-10 h-5 rounded-full relative transition-colors-fast disabled:opacity-60 disabled:cursor-not-allowed"
      style={{
        backgroundColor: checked ? "var(--accent)" : "var(--surface-raised)",
      }}
    >
      <span
        className="absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform-fast"
        style={{
          left: checked ? "calc(100% - 1.125rem)" : "0.125rem",
        }}
      />
    </button>
  );
}
