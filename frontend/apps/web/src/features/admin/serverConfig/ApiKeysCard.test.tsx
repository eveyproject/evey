import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { ApiKeysCard } from "./ApiKeysCard";

const deleteMutate = vi.fn();
const setMutate = vi.fn();
vi.mock("./hooks", () => ({
  useSetConfigEntry: () => ({ mutate: setMutate, isPending: false }),
  useDeleteConfigEntry: () => ({ mutate: deleteMutate, isPending: false }),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

beforeEach(() => vi.clearAllMocks());

// A configured key never travels back to the browser: the server sends the
// mask. The card must therefore never save an untouched editor over it, must
// offer Reset only where a built-in default exists, and must not offer to
// "show" a value it does not hold.
describe("ApiKeysCard", () => {
  function tmdbRow() {
    return screen.getByTestId("api-key-row-api.tmdb_key");
  }

  it("a masked key: custom badge, no reveal button, reset available on the key with a default", () => {
    render(<ApiKeysCard entries={[{ key: "api.tmdb_key", value: "••••••••" }]} />);
    const row = tmdbRow();
    expect(within(row).getByText("admin.serverConfig.apiKeys.usingCustom")).toBeInTheDocument();
    expect(within(row).queryByRole("button", { name: "admin.serverConfig.show" })).toBeNull();
    fireEvent.click(within(row).getByRole("button", { name: "admin.serverConfig.apiKeys.resetToDefault" }));
    expect(deleteMutate).toHaveBeenCalledWith("api.tmdb_key", expect.anything());
  });

  it("saving an untouched or mask-echoing editor never overwrites the credential", () => {
    render(<ApiKeysCard entries={[{ key: "api.tmdb_key", value: "••••••••" }]} />);
    const row = tmdbRow();
    fireEvent.click(within(row).getByRole("button", { name: "common.edit" }));
    fireEvent.click(within(row).getByRole("button", { name: "common.save" }));
    expect(setMutate).not.toHaveBeenCalled();
    expect(within(row).queryByRole("button", { name: "common.save" })).toBeNull();

    fireEvent.click(within(row).getByRole("button", { name: "common.edit" }));
    const input = within(row).getByRole("textbox") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(setMutate).not.toHaveBeenCalled();
  });

  it("a typed key is saved on Enter, Escape abandons the edit", () => {
    render(<ApiKeysCard entries={[]} />);
    const row = tmdbRow();
    expect(within(row).getByText("admin.serverConfig.apiKeys.usingDefault")).toBeInTheDocument();
    fireEvent.click(within(row).getByRole("button", { name: "common.edit" }));
    const input = within(row).getByRole("textbox") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "new-tmdb-key-1234" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(setMutate).toHaveBeenCalledWith({ key: "api.tmdb_key", value: "new-tmdb-key-1234" }, expect.anything());

    fireEvent.click(within(row).getByRole("button", { name: "common.edit" }));
    fireEvent.keyDown(within(row).getByRole("textbox"), { key: "Escape" });
    expect(within(row).queryByRole("textbox")).toBeNull();
  });

  it("a legacy unmasked value can be revealed, and a key without a default shows no reset", () => {
    render(<ApiKeysCard entries={[{ key: "api.omdb_key", value: "abcd1234efgh5678" }]} />);
    const row = screen.getByTestId("api-key-row-api.omdb_key");
    expect(within(row).getByText("abcd••••••5678")).toBeInTheDocument();
    fireEvent.click(within(row).getByRole("button", { name: "admin.serverConfig.show" }));
    expect(within(row).getByText("abcd1234efgh5678")).toBeInTheDocument();
    expect(within(row).queryByRole("button", { name: "admin.serverConfig.apiKeys.resetToDefault" })).toBeNull();
  });
});

describe("ApiKeysCard explanations", () => {
  it("gives every key an 'i' explaining what the service is for", () => {
    render(<ApiKeysCard entries={[]} />);
    const infos = screen.getAllByRole("button", { name: "admin.settingsLayout.moreInfo" });
    expect(infos).toHaveLength(4);
    fireEvent.click(infos[0]);
    expect(screen.getByRole("dialog")).toHaveTextContent("admin.settingsHelp.apiKeyTmdb");
  });
});
