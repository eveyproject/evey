import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", async () => (await import("../../../test/rnDom")).rnDom);
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, o?: unknown) => {
      if (typeof o === "string") return o;
      if (o && typeof o === "object" && "label" in o) return `${key}:${String((o as { label: unknown }).label)}`;
      return key;
    },
  }),
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: new Proxy({}, { get: () => "#000" }), radii: { sm: 4, md: 8, lg: 12 } }),
}));

import { ParserTokensCard } from "./ParserTokensCard";

const infoButtons = (root: HTMLElement) => root.querySelectorAll('[data-testid="setting-info-button"]');

const saveEntry = vi.fn();
beforeEach(() => saveEntry.mockClear());

describe("ParserTokensCard", () => {
  it("seeds the fields from the persisted parser.config blob", () => {
    render(
      <ParserTokensCard
        entries={[{ key: "parser.config", value: '{"extra_qualities":["hdrip","hdr"],"extra_edition_suffixes":["imax"]}' }]}
        saveEntry={saveEntry}
        saving={false}
      />,
    );
    expect((screen.getByTestId("parser-tokens-qualities") as HTMLInputElement).value).toBe("hdrip, hdr");
    expect((screen.getByTestId("parser-tokens-editions") as HTMLInputElement).value).toBe("imax");
  });

  it("saves a normalized JSON blob (trimmed, empties dropped)", () => {
    render(<ParserTokensCard entries={[]} saveEntry={saveEntry} saving={false} />);
    fireEvent.change(screen.getByTestId("parser-tokens-qualities"), { target: { value: " hdrip , , web-dl " } });
    fireEvent.click(screen.getByTestId("parser-tokens-save-btn"));

    expect(saveEntry).toHaveBeenCalledWith(
      {
        key: "parser.config",
        value: JSON.stringify({ extra_qualities: ["hdrip", "web-dl"], extra_edition_suffixes: [] }),
      },
      expect.anything(),
    );
  });

  it("does nothing until a field was edited", () => {
    render(<ParserTokensCard entries={[]} saveEntry={saveEntry} saving={false} />);
    fireEvent.click(screen.getByTestId("parser-tokens-save-btn"));
    expect(saveEntry).not.toHaveBeenCalled();
  });

  it("gives the qualities and editions rows exactly one 'i' button each, explained by the settings help", () => {
    const { container } = render(<ParserTokensCard entries={[]} saveEntry={saveEntry} saving={false} />);
    expect(infoButtons(container).length).toBe(2);
    fireEvent.click(screen.getByRole("button", { name: "admin.settingsLayout.moreInfo:Extra quality tokens" }));
    expect(screen.getByRole("dialog").textContent).toContain("admin.settingsHelp.parserQualities");
    fireEvent.click(screen.getByTestId("setting-info-close"));
    fireEvent.click(screen.getByRole("button", { name: "admin.settingsLayout.moreInfo:Extra edition suffixes" }));
    expect(screen.getByRole("dialog").textContent).toContain("admin.settingsHelp.parserEditions");
  });
});
