import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import React from "react";

vi.mock("react-native", async () => ({ ...(await import("../../../test/rnDom")).rnDom, Linking: { openURL: vi.fn() } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: new Proxy({}, { get: () => "#000" }), radii: { sm: 4, md: 8, lg: 12 } }),
}));

import { ApiKeysCard, API_KEY_FIELDS } from "./ApiKeysCard";

const saveEntry = vi.fn();
const deleteEntry = vi.fn();

function mount(values: Record<string, string> = {}) {
  render(<ApiKeysCard getEntryValue={(k) => values[k] ?? ""} saveEntry={saveEntry} deleteEntry={deleteEntry} saving={false} />);
}

beforeEach(() => vi.clearAllMocks());

describe("ApiKeysCard (mobile)", () => {
  it("gives each provider key its row and its 'i'", () => {
    mount();
    for (const f of API_KEY_FIELDS) {
      expect(within(screen.getByTestId(`api-key-row-${f.dbKey}`)).getAllByTestId("setting-info-button")).toHaveLength(1);
    }
  });

  it("saves a typed key", () => {
    mount();
    fireEvent.change(screen.getByTestId("api-key-input-api.omdb_key"), { target: { value: "abc123" } });
    fireEvent.click(screen.getByTestId("api-key-save-api.omdb_key"));
    expect(saveEntry).toHaveBeenCalledWith({ key: "api.omdb_key", value: "abc123" });
  });

  // The server returns a configured key as the mask: saving it back, or an
  // emptied field, would wipe a credential nobody can read back.
  it("never writes the mask or an empty field; Reset removes the key", () => {
    mount({ "api.tidb_key": "••••••••" });
    fireEvent.click(screen.getByTestId("api-key-save-api.tidb_key"));
    fireEvent.change(screen.getByTestId("api-key-input-api.tidb_key"), { target: { value: "" } });
    fireEvent.click(screen.getByTestId("api-key-save-api.tidb_key"));
    expect(saveEntry).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("api-key-reset-api.tidb_key"));
    expect(deleteEntry).toHaveBeenCalledWith("api.tidb_key");
  });
});
