import { describe, it, expect } from "vitest";
import { parseBlockedIssues } from "./pluginConfigUtils";

// Shapes taken from internal/app/plugin/install.go - the wire format is a Go
// error string, so this parser is only as good as the cases it has seen.
describe("parseBlockedIssues", () => {
  it("extrait les constats d'une analyse critique", () => {
    const msg = "rpc error: code = FailedPrecondition desc = plugin blocked: critical security issues found: [eval-usage child-process]";
    expect(parseBlockedIssues(msg)).toEqual(["eval-usage", "child-process"]);
  });

  it("rend le motif brut pour les refus sans liste", () => {
    const msg = "plugin blocked: theme.js contains forbidden pattern: fetch(";
    expect(parseBlockedIssues(msg)).toEqual(["theme.js contains forbidden pattern: fetch("]);
  });

  it("ne rend rien quand le message parle d'autre chose", () => {
    expect(parseBlockedIssues("connection refused")).toEqual([]);
    expect(parseBlockedIssues("")).toEqual([]);
  });

  // A blocked install with an empty list must not render an empty warning box
  // that says nothing.
  it("ne rend rien pour une liste vide", () => {
    expect(parseBlockedIssues("plugin blocked: critical security issues found: []")).toEqual([]);
  });
});
