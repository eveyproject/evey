import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";

// Static guard: the client-side parental rating table lives ONCE.
//
// web, mobile and TV used to each carry their own copy of the country ->
// minimum-age table and of the two functions that read it (minAgeFor,
// isAgeExceeded). This project's parental regressions have all had the same
// shape: a rule applied on one path and forgotten on its twin (the admin
// bypass fixed in library_handler.go and missed in playback_handler.go for
// two months; DEFAULT_TYPE_FILTER copied to mobile and TV without
// DEDICATED_ROUTES). The three copies were extracted into @evey/client
// (packages/client/src/hooks/useParentalGate.ts) on 2026-09-21; the app
// modules are re-exports of it.
//
// This guard fails the moment a copy of the table grows back in an app: a
// table edited on one client alone would mean the same title asks for a PIN
// on one client and not on another - and, because the server is
// authoritative and already fails closed, the visible symptom would be a PIN
// prompt on all-ages content rather than an exposure, which is exactly the
// kind of thing that gets lived with instead of reported.

const FRONTEND = join(__dirname, "..", "..", "..");
const APP_GATE = (app: string) => join(FRONTEND, "apps", app, "src", "hooks", "useParentalGate.ts");
const SHARED_GATE = join(FRONTEND, "packages", "client", "src", "hooks", "useParentalGate.ts");

const RULES: Record<string, RegExp> = {
  "the country -> minimum-age table": /^(export )?const BY_COUNTRY/m,
  "minAgeFor()": /^(export )?function minAgeFor/m,
  "isAgeExceeded()": /^(export )?function isAgeExceeded/m,
};

describe("the client parental rating table lives once, in @evey/client", () => {
  const shared = readFileSync(SHARED_GATE, "utf8");

  for (const [label, pattern] of Object.entries(RULES)) {
    it(`${label} is defined in the shared module`, () => {
      // A rename or refactor that makes the rule unfindable must fail here
      // rather than pass by finding it nowhere.
      expect(shared, `${label} not found in packages/client useParentalGate.ts`).toMatch(pattern);
    });
  }

  for (const app of ["web", "mobile", "tv"] as const) {
    it(`${app} re-exports the shared gate and carries no copy of the rules`, () => {
      const source = readFileSync(APP_GATE(app), "utf8");
      expect(source).toMatch(/from "@evey\/client"/);
      for (const [label, pattern] of Object.entries(RULES)) {
        expect(
          source,
          `${label} has a local copy in ${app}. A parental rule changed on one client only is how ` +
            `this project has shipped every one of its parental regressions: edit the shared module ` +
            `in packages/client and import it everywhere.`,
        ).not.toMatch(pattern);
      }
    });
  }
});
