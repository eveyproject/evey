import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { describe, it, expect } from "vitest";

// The TV audit, as a test suite rather than a report.
//
// A report is read once and rots. These are the rules the television actually
// imposes, encoded so that breaking one fails the build with the file and the
// line - the same shape as the i18n guard next door, and for the same reason:
// every rule below has already been broken at least once in this app.
//
// Each check carries an ALLOWLIST of known offenders rather than being
// switched off. That keeps the debt visible and countable: the list may only
// ever shrink, and a new file cannot join it silently.

function repoRoot(): string {
  let dir = resolve(process.cwd());
  for (let i = 0; i < 8; i++) {
    if (existsSync(join(dir, "i18n", "en"))) return dir;
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  throw new Error("repo root not found above " + process.cwd());
}

const APP_ROOT = join(repoRoot(), "frontend", "apps", "tv");

function sourceFiles(dir: string, acc: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (["node_modules", "android", "ios", ".expo", "build", "dist"].includes(entry)) continue;
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) sourceFiles(full, acc);
    else if (entry.endsWith(".tsx") && !entry.includes(".test.")) acc.push(full);
  }
  return acc;
}

const rel = (f: string) => relative(APP_ROOT, f);
const screens = () => sourceFiles(join(APP_ROOT, "app"));
const components = () => sourceFiles(join(APP_ROOT, "src"));

// --- 1. Remote: initial focus must be deterministic --------------------------
//
// Several elements claiming hasTVPreferredFocus unconditionally on the same
// screen leaves the framework to pick, so the viewer's first press goes
// somewhere different depending on mount order. Conditional claims
// (`hasTVPreferredFocus={index === 0}`, `={focusFirst}`) are fine: at most one
// is true at a time.
describe("audit TV · focus initial déterministe", () => {
  // Screens that legitimately declare more than one because the branches are
  // mutually exclusive early-returns (disabled state / empty state / content).
  const ALLOW = new Set<string>([
    "app/(auth)/(tabs)/requests.tsx", // état désactivé + onglets + lignes de liste
    "app/(auth)/(tabs)/search.tsx",
    "app/(auth)/(tabs)/library.tsx",
    "app/(auth)/(tabs)/music.tsx",
    "app/(auth)/settings/playback.tsx",
    "app/(auth)/photos/index.tsx",
    "app/(auth)/genres/index.tsx",
    "app/(auth)/genres/[genre].tsx",
    "app/(auth)/library/[libraryId].tsx",
    "app/(auth)/playlists/index.tsx",
    "app/(auth)/collections/index.tsx",
    "app/(auth)/history.tsx",
    // Le player : l'un des deux est dans le retour anticipé « session
    // expirée », l'autre dans la barre de contrôle du lecteur - ils ne
    // coexistent jamais. Un regex ne sait pas lire un early return ; c'est la
    // limite assumée de cette règle, pas un défaut du player.
    "app/(auth)/player/[id].tsx",
  ]);

  it("aucun écran ne réclame le focus initial deux fois sans condition", () => {
    const offenders: string[] = [];
    for (const file of screens()) {
      const name = rel(file);
      if (ALLOW.has(name)) continue;
      const src = readFileSync(file, "utf8");
      // Unconditional forms only: bare `hasTVPreferredFocus` or `={true}`.
      const unconditional =
        (src.match(/hasTVPreferredFocus(?!\s*=)/g) ?? []).length +
        (src.match(/hasTVPreferredFocus=\{true\}/g) ?? []).length;
      if (unconditional > 1) offenders.push(`${name} (${unconditional})`);
    }
    expect(offenders.sort()).toEqual([]);
  });

  it("la liste des exceptions ne contient que des fichiers existants", () => {
    // A stale allowlist silently weakens the rule; this is what makes it
    // shrink instead of drift.
    for (const name of ALLOW) {
      expect(existsSync(join(APP_ROOT, name)), `${name} n'existe plus, retire-le de ALLOW`).toBe(true);
    }
  });
});

// --- 2. Remote: every interactive element announces itself -------------------
//
// A screen reader on a television reads the focused element and nothing else -
// there is no surrounding context to fall back on, so an unlabelled button is
// announced as "button" and means nothing.
describe("audit TV · libellés d'accessibilité", () => {
  it("chaque FocusableCard porte un accessibilityLabel", () => {
    const offenders: string[] = [];
    for (const file of [...screens(), ...components()]) {
      const src = readFileSync(file, "utf8");
      // Split on the opening tag so each element is examined on its own.
      const blocks = src.split("<FocusableCard").slice(1);
      blocks.forEach((block, i) => {
        const head = block.slice(0, block.indexOf(">") + 1);
        // A card whose only child is text is self-describing for the reader;
        // the rule targets the ones wrapping icons or images.
        if (/accessibilityLabel/.test(head)) return;
        if (/accessible=\{false\}/.test(head)) return;
        offenders.push(`${rel(file)} #${i + 1}`);
      });
    }
    // A ratchet, not a target: 68 is what the app carries today. Fixing cards
    // lowers it; this number may only ever go DOWN. Raising it to make a build
    // pass is the one move that defeats the check.
    expect(offenders.length).toBeLessThanOrEqual(68);
  });
});

// --- 3. Resilience: a failed request must be sayable -------------------------
//
// The failure this encodes: `if (isLoading || !data) return <spinner/>` spins
// forever when the request errors, because isLoading is false and data is
// undefined. On a television that is indistinguishable from a frozen app.
describe("audit TV · échec de requête visible", () => {
  const ALLOW = new Set<string>([
    // Écrans à onglets : la barre reste focusable, l'utilisateur n'est pas
    // piégé, et un état vide y est plausible. À traiter, mais moins grave.
    "app/(auth)/(tabs)/index.tsx",
    "app/(auth)/(tabs)/requests.tsx",
    "app/(auth)/(tabs)/settings.tsx",
    "app/(auth)/(tabs)/music.tsx",
    // Écrans profonds restants : même correctif que media/[id], pas encore fait.
    "app/(auth)/settings/playback.tsx",
    "app/(auth)/settings/stats.tsx",
    "app/(auth)/music/album/[id].tsx",
    "app/(auth)/music/artist/[id].tsx",
    "app/(auth)/person/[id].tsx",
    // Écrans d'entrée : gèrent leurs erreurs par toast/dialogue, pas par état.
    "app/login.tsx",
    "app/profile-select.tsx",
    "app/quick-connect.tsx",
  ]);

  it("un écran qui interroge le serveur sait dire que ça a échoué", () => {
    const offenders: string[] = [];
    for (const file of screens()) {
      const name = rel(file);
      if (ALLOW.has(name)) continue;
      const src = readFileSync(file, "utf8");
      const fetches = /useQuery|use[A-Z][A-Za-z]+Service[A-Z]/.test(src);
      if (!fetches) continue;
      // `<ScreenState`, not `ScreenState`: matching the import let a screen
      // keep the import while deleting the usage, and the check stayed green
      // through a deliberately reintroduced regression.
      if (!/isError|<ScreenState/.test(src)) offenders.push(name);
    }
    expect(offenders.sort()).toEqual([]);
  });

  it("la liste des exceptions ne contient que des fichiers existants", () => {
    for (const name of ALLOW) {
      expect(existsSync(join(APP_ROOT, name)), `${name} n'existe plus, retire-le de ALLOW`).toBe(true);
    }
  });
});

// --- 4. tvOS: nothing Android-only without a guard ---------------------------
//
// The TV app ships to Apple TV too. A native module that only exists on
// Android must be reached through an optional lookup, or the tvOS build throws
// on import - and the failure lands at startup, far from the cause.
describe("audit TV · parité tvOS", () => {
  const ANDROID_ONLY = ["@evey/media-caps", "@evey/voice-search", "@evey/offline-audio"];

  it("tout module natif Android-only passe par une résolution optionnelle", () => {
    const offenders: string[] = [];
    for (const pkg of ANDROID_ONLY) {
      for (const file of [...screens(), ...components(), ...sourceFiles(join(APP_ROOT, "src"))]) {
        const src = readFileSync(file, "utf8");
        if (!src.includes(pkg)) continue;
        // The package's own entry point is what must degrade; a consumer just
        // has to not assume the result exists.
        const entry = join(repoRoot(), "frontend", "packages", pkg.replace("@evey/", ""), "src", "index.ts");
        if (!existsSync(entry)) continue;
        const entrySrc = readFileSync(entry, "utf8");
        if (!/requireOptionalNativeModule|Platform\.OS|try\s*\{/.test(entrySrc)) {
          offenders.push(`${pkg} (importé par ${rel(file)})`);
        }
      }
    }
    expect([...new Set(offenders)].sort()).toEqual([]);
  });

  it("aucun écran ne suppose Android sans traiter l'autre plateforme", () => {
    const offenders: string[] = [];
    for (const file of [...screens(), ...components()]) {
      const src = readFileSync(file, "utf8");
      if (!/Platform\.OS\s*===\s*["']android["']/.test(src)) continue;
      // Testing for Android is fine; assuming the else branch is impossible is
      // not. A file that mentions ios/Apple TV has thought about it.
      if (!/["']ios["']|Apple TV/.test(src)) offenders.push(rel(file));
    }
    expect(offenders.sort()).toEqual([]);
  });
});
