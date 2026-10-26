import { beforeEach, describe, expect, it, vi } from "vitest";

const files = vi.hoisted(() => new Map<string, string>());
vi.mock("expo-file-system/legacy", () => ({
  cacheDirectory: "file:///cache/",
  getInfoAsync: async (p: string) => ({ exists: files.has(p) }),
  readAsStringAsync: async (p: string) => files.get(p) ?? "",
  writeAsStringAsync: async (p: string, v: string) => void files.set(p, v),
  makeDirectoryAsync: async () => {},
}));

import { readCatalogue, writeCatalogue } from "./i18nCatalogueCache";

describe("i18nCatalogueCache", () => {
  beforeEach(() => files.clear());

  it("hands back the catalogue last written for that server and language only", async () => {
    await writeCatalogue("https://tv.example:9091", "fr", { "common.ok": "D'accord" });
    expect(await readCatalogue("https://tv.example:9091", "fr")).toEqual({ "common.ok": "D'accord" });
    expect(await readCatalogue("https://tv.example:9091", "de")).toBeNull();
    expect(await readCatalogue("https://other.example", "fr")).toBeNull();
  });

  it("a corrupt file or no server is a miss, never a throw", async () => {
    await writeCatalogue("https://tv.example", "fr", { a: "b" });
    for (const k of files.keys()) files.set(k, "{broken");
    expect(await readCatalogue("https://tv.example", "fr")).toBeNull();
    expect(await readCatalogue("", "fr")).toBeNull();
  });
});
