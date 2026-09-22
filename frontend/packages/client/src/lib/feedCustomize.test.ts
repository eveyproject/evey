import { describe, it, expect } from "vitest";
import { buildFeedRows, moveRow, toggleRow } from "./feedCustomize";

const libs = { "lib-1": "Films" };
const plugins = { "plg-a": "weather" };

describe("buildFeedRows", () => {
  it("garde les types connus et les bibliotheques qui existent", () => {
    const rows = buildFeedRows(
      [
        { feedType: "most_popular", enabled: true, displayOrder: 0 },
        { feedType: "library:lib-1", enabled: false, displayOrder: 1 },
      ],
      libs,
      {},
    );
    expect(rows.map((r) => r.feedType)).toEqual(["most_popular", "library:lib-1"]);
    expect(rows[1].enabled).toBe(false);
  });

  // The defect: a feed whose library or plugin is gone rendered as a raw id
  // with no way to get rid of it.
  it("jette les lignes orphelines", () => {
    const rows = buildFeedRows(
      [
        { feedType: "library:disparue", enabled: true, displayOrder: 0 },
        { feedType: "plugin:plg-supprime", enabled: true, displayOrder: 1 },
        { feedType: "most_popular", enabled: true, displayOrder: 2 },
      ],
      libs,
      plugins,
    );
    // plg-a is installed but absent from the config, so it is prepended; what
    // matters here is that the two dead rows are gone.
    expect(rows.map((r) => r.feedType)).toEqual(["plugin:plg-a", "most_popular"]);
  });

  it("fait remonter un plugin fraichement installe en tete", () => {
    const rows = buildFeedRows([{ feedType: "most_popular", enabled: true, displayOrder: 0 }], libs, plugins);
    expect(rows.map((r) => r.feedType)).toEqual(["plugin:plg-a", "most_popular"]);
  });

  it("ne duplique pas un plugin deja present dans la config", () => {
    const rows = buildFeedRows(
      [{ feedType: "plugin:plg-a", enabled: false, displayOrder: 0 }],
      libs,
      plugins,
    );
    expect(rows).toHaveLength(1);
    expect(rows[0].enabled).toBe(false);
  });

  // displayOrder must be the position, not whatever was stored before a row
  // was dropped - otherwise the next save writes gaps back to the server.
  it("renumerote toujours a partir de zero", () => {
    const rows = buildFeedRows(
      [
        { feedType: "recommended", enabled: true, displayOrder: 7 },
        { feedType: "most_popular", enabled: true, displayOrder: 42 },
      ],
      {},
      {},
    );
    expect(rows.map((r) => r.displayOrder)).toEqual([0, 1]);
  });
});

describe("moveRow", () => {
  const rows = buildFeedRows(
    [
      { feedType: "recommended", enabled: true, displayOrder: 0 },
      { feedType: "most_popular", enabled: true, displayOrder: 1 },
      { feedType: "collections", enabled: true, displayOrder: 2 },
    ],
    {},
    {},
  );

  it("echange avec le voisin et renumerote", () => {
    const moved = moveRow(rows, 1, -1);
    expect(moved.map((r) => r.feedType)).toEqual(["most_popular", "recommended", "collections"]);
    expect(moved.map((r) => r.displayOrder)).toEqual([0, 1, 2]);
  });

  it("ne fait rien aux extremites", () => {
    expect(moveRow(rows, 0, -1).map((r) => r.feedType)).toEqual(rows.map((r) => r.feedType));
    expect(moveRow(rows, 2, 1).map((r) => r.feedType)).toEqual(rows.map((r) => r.feedType));
  });
});

describe("toggleRow", () => {
  it("ne retourne que la ligne visee", () => {
    const rows = buildFeedRows(
      [
        { feedType: "recommended", enabled: true, displayOrder: 0 },
        { feedType: "most_popular", enabled: true, displayOrder: 1 },
      ],
      {},
      {},
    );
    const next = toggleRow(rows, 1);
    expect(next[0].enabled).toBe(true);
    expect(next[1].enabled).toBe(false);
  });
});
