import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { SmtpSettingsCard } from "./SmtpSettingsCard";

const setMutate = vi.fn();
vi.mock("./hooks", () => ({ useSetConfigEntry: () => ({ mutate: setMutate, isPending: false }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}(${Object.values(o).join(",")})` : k) }) }));
vi.mock("@/components/ui/Select", () => ({
  Select: ({ label, value, onChange, options }: any) =>
    React.createElement("select", { "aria-label": label, value, onChange: (e: React.ChangeEvent<HTMLSelectElement>) => onChange(e.target.value) }, options.map((o: any) => React.createElement("option", { key: o.value, value: o.value }, o.label))),
}));

const stored = [
  { key: "smtp.host", value: "mail.example.test" },
  { key: "smtp.port", value: "465" },
  { key: "smtp.username", value: "evey" },
  { key: "smtp.password", value: "s3cret" },
  { key: "smtp.from", value: "evey@example.test" },
  { key: "smtp.tls", value: "ssl" },
];

beforeEach(() => vi.clearAllMocks());

// The six rows are seeded ONCE the config arrives - a card mounted before the
// query resolved used to save blanks over the stored settings - and never
// re-seeded over an edit in progress; Save writes all six and reports once
// only when every write landed, naming the row that failed otherwise.
describe("SmtpSettingsCard", () => {
  it("seeds once the config arrives and does not clobber an edit on a refetch", () => {
    const { rerender } = render(<SmtpSettingsCard entries={undefined} />);
    const host = screen.getByLabelText("admin.serverConfig.smtpHost") as HTMLInputElement;
    expect(host.value).toBe("");
    expect((screen.getByLabelText("admin.serverConfig.port") as HTMLInputElement).value).toBe("587");
    rerender(<SmtpSettingsCard entries={stored} />);
    expect(host.value).toBe("mail.example.test");
    expect((screen.getByLabelText("admin.serverConfig.tlsMode") as HTMLSelectElement).value).toBe("ssl");
    fireEvent.change(host, { target: { value: "smtp.new.test" } });
    rerender(<SmtpSettingsCard entries={[...stored, { key: "smtp.host", value: "refetched" }]} />);
    expect(host.value).toBe("smtp.new.test");
  });

  it("saves all six rows and reports once, or names the failed row", () => {
    render(<SmtpSettingsCard entries={stored} />);
    fireEvent.click(screen.getByText("admin.serverConfig.saveSmtp"));
    expect(setMutate).toHaveBeenCalledTimes(6);
    expect(setMutate.mock.calls.map((c) => c[0])).toEqual(stored);
    const cbs = setMutate.mock.calls.map((c) => c[1] as { onSuccess: () => void; onError: () => void });
    cbs.slice(0, 5).forEach((cb) => cb.onSuccess());
    expect(toast.success).not.toHaveBeenCalled();
    cbs[5].onSuccess();
    expect(toast.success).toHaveBeenCalledTimes(1);

    vi.clearAllMocks();
    fireEvent.click(screen.getByText("admin.serverConfig.saveSmtp"));
    const again = setMutate.mock.calls.map((c) => c[1] as { onSuccess: () => void; onError: () => void });
    again[3].onError();
    expect(toast.error).toHaveBeenCalledWith("admin.serverConfig.failedSmtp(smtp.password)");
    again.filter((_, i) => i !== 3).forEach((cb) => cb.onSuccess());
    expect(toast.success).not.toHaveBeenCalled();
  });
});
