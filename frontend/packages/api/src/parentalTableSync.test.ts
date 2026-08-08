import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";

// Static guard: the three client-side parental rating tables must stay identical.
//
// web, mobile and TV each carry their own copy of the country -> minimum-age
// table and of the two functions that read it (minAgeFor, isAgeExceeded), in
// their own useParentalGate.ts. The copies are byte-identical today. Nothing
// keeps them that way.
//
// This project's parental regressions have all had the same shape: a rule
// applied on one path and forgotten on its twin. It happened with the admin
// bypass (fixed in library_handler.go, missed in playback_handler.go for two
// months) and with the shared library rules (mobile and TV copied
// DEFAULT_TYPE_FILTER but not DEDICATED_ROUTES). A table edited on web alone
// would mean the same title asks for a PIN on one client and not on another -
// and, because the server is authoritative and already fails closed, the
// visible symptom would be a PIN prompt on all-ages content rather than an
// exposure, which is exactly the kind of thing that gets lived with instead of
// reported.
//
// The guard deliberately compares TEXT rather than behaviour, and does not ask
// for the tables to be extracted into this package. Extraction is the better
// end state, but it edits three parental files; this test costs nothing and
// fails the moment they diverge. Whoever changes one is told to change all
// three - or to do the extraction properly.

const APPS = join(__dirname, "..", "..", "..", "apps");
const GATE = (app: string) => join(APPS, app, "src", "hooks", "useParentalGate.ts");

/** Text of a top-level block, from its opening line to a closing brace in column 0. */
function block(source: string, startPattern: RegExp): string {
  const lines = source.split("\n");
  const start = lines.findIndex((l) => startPattern.test(l));
  if (start < 0) return "";
  for (let i = start; i < lines.length; i++) {
    if (i > start && /^[};]/.test(lines[i])) {
      // `export ` is incidental: web keeps these module-private, mobile and TV
      // export them. Only the rule itself has to match.
      return lines.slice(start, i + 1).join("\n").replace(/^export /gm, "");
    }
  }
  return "";
}

const REGIONS: Record<string, RegExp> = {
  "the country -> minimum-age table": /^(export )?const BY_COUNTRY/,
  "minAgeFor()": /^(export )?function minAgeFor/,
  "isAgeExceeded()": /^(export )?function isAgeExceeded/,
};

describe("the client parental rating tables stay in sync across apps", () => {
  const sources = {
    web: readFileSync(GATE("web"), "utf8"),
    mobile: readFileSync(GATE("mobile"), "utf8"),
    tv: readFileSync(GATE("tv"), "utf8"),
  };

  for (const [label, pattern] of Object.entries(REGIONS)) {
    it(`${label} is the same on web, mobile and TV`, () => {
      const web = block(sources.web, pattern);
      // A rename or refactor that makes the block unfindable must fail here
      // rather than pass by comparing two empty strings.
      expect(web, `${label} not found in web's useParentalGate.ts`).not.toBe("");

      for (const app of ["mobile", "tv"] as const) {
        expect(
          block(sources[app], pattern),
          `${label} differs between web and ${app}. A parental rule changed on one client only ` +
            `is how this project has shipped every one of its parental regressions: change all ` +
            `three, or move the rule into @evey/api and import it everywhere.`,
        ).toBe(web);
      }
    });
  }
});
