import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ParserTokensCard } from "./ParserTokensCard";

const setMutate = vi.fn();
vi.mock("./hooks", () => ({ useSetConfigEntry: () => ({ mutate: setMutate, isPending: false }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, d?: string) => d ?? _k }) }));

beforeEach(() => vi.clearAllMocks());

// The parser's extra tokens live as one JSON blob: the card seeds the two
// comma lists from it (a corrupt blob reads as empty rather than crashing),
// saves trimmed non-empty tokens back as JSON, and re-syncs when the stored
// value changes under it.
describe("ParserTokensCard", () => {
  it("seeds from the blob, saves trimmed tokens as JSON, and re-syncs on an external change", () => {
    const { rerender } = render(<ParserTokensCard entries={[{ key: "parser.config", value: JSON.stringify({ extra_qualities: ["hdrip"], extra_edition_suffixes: ["imax"] }) }]} />);
    const q = screen.getByTestId("parser-tokens-qualities") as HTMLTextAreaElement;
    const e = screen.getByTestId("parser-tokens-editions") as HTMLTextAreaElement;
    expect(q.value).toBe("hdrip");
    expect(e.value).toBe("imax");
    const save = screen.getByTestId("parser-tokens-save-btn") as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    fireEvent.change(q, { target: { value: " hdrip , web-dl,, " } });
    fireEvent.change(e, { target: { value: "" } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);
    expect(setMutate).toHaveBeenCalledWith({ key: "parser.config", value: JSON.stringify({ extra_qualities: ["hdrip", "web-dl"], extra_edition_suffixes: [] }) });
    expect(save.disabled).toBe(true);

    rerender(<ParserTokensCard entries={[{ key: "parser.config", value: JSON.stringify({ extra_qualities: ["remux"] }) }]} />);
    expect(q.value).toBe("remux");
    expect(e.value).toBe("");
  });

  it("a corrupt or missing blob reads as empty lists", () => {
    render(<ParserTokensCard entries={[{ key: "parser.config", value: "{not json" }]} />);
    expect((screen.getByTestId("parser-tokens-qualities") as HTMLTextAreaElement).value).toBe("");
    render(<ParserTokensCard entries={undefined} />);
    expect((screen.getAllByTestId("parser-tokens-editions").at(-1) as HTMLTextAreaElement).value).toBe("");
  });
});
