import { OptionGroup } from "../settings/OptionGroup";

export interface ChipOption {
  value: string;
  label: string;
}

interface ChipSelectProps {
  options: ChipOption[];
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  /** Accessible name of the choice (the setting it picks). */
  label?: string;
  testID?: string;
}

// ChipSelect is the short-enum picker of the server-config cards (memory
// profile, mkvgo modes, SMTP TLS mode, CPU shares). It used to be a wrapping
// row of pills, which left a ragged 2+1 grid on a phone; it is now the
// equal-width OptionGroup, kept under this name for the cards that use it.
export function ChipSelect({ options, value, onChange, disabled, label, testID }: ChipSelectProps) {
  return <OptionGroup label={label ?? ""} options={options} value={value} onChange={onChange} disabled={disabled} testID={testID} />;
}
