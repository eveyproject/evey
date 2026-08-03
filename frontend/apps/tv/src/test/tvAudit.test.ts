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

// Returns null rather than throwing: the CI container mounts only `frontend`
// at /app, so the repo root is genuinely out of reach there. These checks then
// SKIP visibly - `make pre-push` runs them on the host, which is where they
// are actually enforced.
function repoRoot(): string | null {
  let dir = resolve(process.cwd());
  for (let i = 0; i < 8; i++) {
    if (existsSync(join(dir, "i18n", "en"))) return dir;
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  return null;
}

const REPO_ROOT = repoRoot();
const CAN_RUN = REPO_ROOT !== null;
const APP_ROOT = join(REPO_ROOT ?? "", "frontend", "apps", "tv");

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

// End of a JSX opening tag, counted properly.
//
// `indexOf(">")` looked obvious and was wrong: `onPress={() => x()}` contains a
// `>`, so the tag was cut short and every prop after the first arrow function
// became invisible. That made this very audit report ScreenState - a component
// whose two cards ARE labelled - as an offender. A rule that miscounts is worse
// than no rule, because the ratchet then locks in a fictional number.
function openingTagEnd(block: string): number {
  let depth = 0;
  let quote = "";
  for (let i = 0; i < block.length; i++) {
    const c = block[i];
    if (quote) {
      if (c === quote) quote = "";
      continue;
    }
    if (c === '"' || c === "'" || c === "`") quote = c;
    else if (c === "{") depth++;
    else if (c === "}") depth--;
    else if (c === ">" && depth === 0) return i + 1;
  }
  return block.length;
}
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

  it.skipIf(!CAN_RUN)("aucun écran ne réclame le focus initial deux fois sans condition", () => {
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

  // The mirror rule, and the one that actually bites: a screen with NO focus
  // claim at all leaves Android with no focused view, and Android then stops
  // delivering key events entirely - the remote goes dead, not just idle. Cost
  // a night on the player and the media page (2026-08-03).
  //
  // Exempt: screens whose SHELL already owns a focus target - the tab screens
  // (the sidebar in (tabs)/_layout renders focusable nav items) and the layouts
  // themselves.
  const SHELL_PROVIDES_FOCUS = new Set<string>([
    "app/(auth)/(tabs)/favorites.tsx",
    "app/(auth)/(tabs)/_layout.tsx",
    "app/(auth)/_layout.tsx",
    "app/_layout.tsx",
    "app/index.tsx",
  ]);

  it.skipIf(!CAN_RUN)("tout écran qui rend un élément focusable réclame le focus initial", () => {
    const offenders: string[] = [];
    for (const file of screens()) {
      const name = rel(file);
      if (SHELL_PROVIDES_FOCUS.has(name)) continue;
      const src = readFileSync(file, "utf8");
      const hasFocusable = /<(Pressable|FocusableCard|TextInput)\b/.test(src);
      if (!hasFocusable) continue;
      if (!src.includes("hasTVPreferredFocus")) offenders.push(name);
    }
    expect(offenders.sort()).toEqual([]);
  });

  // The same trap, one layer up: an overlay component (Modal) that never
  // claims focus leaves Android with nothing focused the moment it opens,
  // dead remote included. Two of these were fixed the same night
  // (AddToPlaylistSheet, FeedCustomizePanel) - this keeps a third from
  // shipping unnoticed.
  const OVERLAY_ALLOW = new Set<string>([]);

  it.skipIf(!CAN_RUN)("tout composant qui rend un Modal réclame le focus initial", () => {
    const offenders: string[] = [];
    for (const file of components()) {
      const name = rel(file);
      if (OVERLAY_ALLOW.has(name)) continue;
      const src = readFileSync(file, "utf8");
      if (!/<Modal\b/.test(src)) continue;
      if (!src.includes("hasTVPreferredFocus")) offenders.push(name);
    }
    expect(offenders.sort()).toEqual([]);
  });

  it.skipIf(!CAN_RUN)("la liste des exceptions ne contient que des fichiers existants", () => {
    // A stale allowlist silently weakens the rule; this is what makes it
    // shrink instead of drift.
    for (const name of [...ALLOW, ...SHELL_PROVIDES_FOCUS, ...OVERLAY_ALLOW]) {
      expect(existsSync(join(APP_ROOT, name)), `${name} n'existe plus, retire-le de ALLOW`).toBe(true);
    }
  });
});

// --- 1b. Music routes are built by the helper, never by hand -----------------
//
// The album screen needs BOTH the album id and its library id: a hand-built
// "/music/album/<id>" left its query disabled, so the screen showed the raw id
// as the title and "no track found" (device, 2026-08-03). musicAlbumHref and
// musicArtistHref are the only correct way to spell those links.
describe("audit TV \u00b7 routes musique", () => {
  it.skipIf(!CAN_RUN)("aucun \u00e9cran ne fabrique une route musique \u00e0 la main", () => {
    const offenders: string[] = [];
    for (const file of [...screens(), ...components()]) {
      const name = rel(file);
      if (name.endsWith("musicRoutes.ts")) continue;
      const src = readFileSync(file, "utf8");
      if (/["\`]\/?\(auth\)\/music\/(album|artist)\//.test(src)) offenders.push(name);
    }
    expect(offenders.sort()).toEqual([]);
  });
});

// --- 2. Remote: every interactive element announces itself -------------------
//
// A screen reader on a television reads the focused element and nothing else -
// there is no surrounding context to fall back on, so an unlabelled button is
// announced as "button" and means nothing.
describe("audit TV · libellés d'accessibilité", () => {
  it.skipIf(!CAN_RUN)("chaque FocusableCard porte un accessibilityLabel", () => {
    const offenders: string[] = [];
    for (const file of [...screens(), ...components()]) {
      const src = readFileSync(file, "utf8");
      // Split on the opening tag so each element is examined on its own.
      const blocks = src.split("<FocusableCard").slice(1);
      blocks.forEach((block, i) => {
        const head = block.slice(0, openingTagEnd(block));
        // A card whose only child is text is self-describing for the reader;
        // the rule targets the ones wrapping icons or images.
        if (/accessibilityLabel/.test(head)) return;
        if (/accessible=\{false\}/.test(head)) return;
        offenders.push(`${rel(file)} #${i + 1}`);
      });
    }
    // Zero. This started as a ratchet at 68, dropped to 42 once the
    // opening-tag scan above stopped being fooled by arrow functions, and is
    // now simply enforced: every card in the app announces itself.
    //
    // Keep it at zero. An exception belongs in the card (`accessible={false}`
    // for something genuinely decorative), stated at the site, not as a number
    // here that quietly grants everyone one free pass.
    expect(offenders.sort()).toEqual([]);
  });
});

// --- 3. Resilience: a failed request must be sayable -------------------------
//
// The failure this encodes: `if (isLoading || !data) return <spinner/>` spins
// forever when the request errors, because isLoading is false and data is
// undefined. On a television that is indistinguishable from a frozen app.
describe("audit TV · échec de requête visible", () => {
  const ALLOW = new Set<string>([
    // Réglages : le SEUL écran qui reste utile serveur muet - thème, langue et
    // adresse du serveur sont locaux et continuent de fonctionner. Le couvrir
    // d'un état d'erreur retirerait des commandes qui marchent, ce qui est pire
    // que le défaut qu'on corrige ailleurs.
    "app/(auth)/(tabs)/settings.tsx",
    // Le user a du travail non commité sur cet écran au 08-02 : on n'y touche
    // pas tant qu'il l'a en cours.
    "app/quick-connect.tsx",
  ]);

  it.skipIf(!CAN_RUN)("un écran qui interroge le serveur sait dire que ça a échoué", () => {
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
      //
      // Alert/toast count as surfacing too. The pattern above also matches
      // MUTATION hooks (useUserServiceLogin), and a form that reports its
      // failure in a dialog is doing the right thing - demanding a screen-level
      // error state there would be the rule bullying the code.
      if (!/isError|<ScreenState|Alert\.alert|toast\.error/.test(src)) offenders.push(name);
    }
    expect(offenders.sort()).toEqual([]);
  });

  it.skipIf(!CAN_RUN)("la liste des exceptions ne contient que des fichiers existants", () => {
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

  it.skipIf(!CAN_RUN)("tout module natif Android-only passe par une résolution optionnelle", () => {
    const offenders: string[] = [];
    for (const pkg of ANDROID_ONLY) {
      for (const file of [...screens(), ...components(), ...sourceFiles(join(APP_ROOT, "src"))]) {
        const src = readFileSync(file, "utf8");
        if (!src.includes(pkg)) continue;
        // The package's own entry point is what must degrade; a consumer just
        // has to not assume the result exists.
        const entry = join(REPO_ROOT ?? "", "frontend", "packages", pkg.replace("@evey/", ""), "src", "index.ts");
        if (!existsSync(entry)) continue;
        const entrySrc = readFileSync(entry, "utf8");
        if (!/requireOptionalNativeModule|Platform\.OS|try\s*\{/.test(entrySrc)) {
          offenders.push(`${pkg} (importé par ${rel(file)})`);
        }
      }
    }
    expect([...new Set(offenders)].sort()).toEqual([]);
  });

  it.skipIf(!CAN_RUN)("aucun écran ne suppose Android sans traiter l'autre plateforme", () => {
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
