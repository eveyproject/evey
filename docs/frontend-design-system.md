# Frontend design system

The rules that keep the three clients (web, mobile, TV) coherent and fast -
and stop one-off patches from accumulating. When a UI bug tempts you to add a
special case, check here first: most classes of bugs already have a canonical
answer, and the fix belongs in the convention (or its utility), not at the
call site.

## Tokens - the single source of truth

`frontend/packages/theme/src/themeVars.ts` is the single source of truth for
colors, radii and spacing. The web CSS custom properties are **generated** from
it (`packages/theme/gen-theme-css.mts` → `apps/web/public/themes/*.css`); the
React Native client reads `tokens.ts`, which is **derived** from the same
`themeVars.ts` and exposed through `useTheme()`. Edit `themeVars.ts`, never the
generated CSS or the derived RN tokens.

- Never hardcode a color. Web: `var(--accent)`, `var(--text-muted)`, etc.
  Native: `theme.colors.accent`.
- Never invent a radius or spacing step; use the token scale.
- A new semantic color (e.g. a badge tint) is added to `ThemeTokens` and every
  theme - not inlined where it is first needed.

## Touch targets (posed 2026-07-11)

Every interactive element must offer a >= 44x44 px hit area (WCAG 2.5.5,
Apple HIG). Two utilities in `apps/web/src/styles/main.css` implement this:

- `.tap-target` - grows the element itself (`min-width/height: 44px`). Use it
  when layout allows the box to grow (toolbar buttons, avatar button).
- `.tap-hit` - keeps the rendered size and extends the HIT AREA with a
  centered pseudo-element. Use it on small absolutely-positioned controls
  where growing the box would shift the design (card badges, checkboxes,
  overflow dots).

New interactive elements pick one of the two. A control smaller than 44px
without one of these classes is a bug.

## Mobile-viewport performance rules

Learned the hard way (07-11 mobile-web perf pass); breaking one of these
reintroduces jank that is very hard to bisect later:

1. **No `backdrop-filter` on mobile-rendered surfaces.** A blur on a sticky
   bar repaints every scrolled frame; one per card multiplies compositor
   layers. Keep glass effects behind `@media (min-width: 768px)` or
   `[@media(hover:hover)]` variants. Fall back to an OPAQUE surface color -
   translucency without blur reads as broken.
2. **Never remount a page to get a transition.** `key={pathname}` on a layout
   node unmounts/remounts the whole page on every navigation. Scroll reset
   and enter-animations are done with a ref effect (`scrollTo` + WAAPI
   `el.animate`), feature-detected for jsdom.
3. **Cap what a stacked page mounts initially.** Horizontal rows render a
   bounded number of items (see `MediaSlider` INITIAL_CARDS) and reveal the
   rest on first interaction; vertical grids virtualize (`useVirtualizer` from
   `@tanstack/react-virtual`, see `MediaGrid`).
4. **Route files must not import their component.** A non-lazy route file is
   statically imported by `routeTree.gen.ts`: anything it pulls in ships to
   every page. Heavy features (hls.js!) live in the `.lazy.tsx` sibling.
   Check after adding a route: `dist/index.html` modulepreload should list
   vendor + i18n only.
5. **Per-item subscriptions select primitives.** A hook consumed by every
   card in a list must `select` a primitive (boolean/string) from its query
   so a refetch re-renders only the items whose value changed - never hand
   each item a freshly-built Map/array (see `useIsWatched` vs
   `useWatchedMap`).
6. **iOS inputs >= 16px** (`text-base`) under `sm`, or Safari zooms on focus.
7. **`dvh`, not `vh`**, for full-height layouts (mobile URL bar), and
   `env(safe-area-inset-bottom)` on fixed bottom bars.

## Established primitives - use them, don't fork them

| Concern | Canonical answer |
|---|---|
| Data fetching | TanStack Query via `@evey/api` generated hooks; raw `fetch` only for streaming bodies (SSE tail, downloads) |
| HTTP client | `eveyFetch` (auth, refresh, CSRF) - never raw fetch for JSON APIs |
| Toasts | `toast` from `@evey/store` (one store, one container) |
| Client state | Zustand stores in `@evey/store`, one concern per store |
| Navigation (web) | `useNavigate`/`Link` from TanStack Router - never `window.location.href` |
| Modals | `components/ui/Modal` (scroll cap + safe-area handled there) |
| Errors in views | `isError` branch with `EmptyState` + `common.errorBoundary.*` keys - never fall through to the empty-state copy |
| Mutations | optimistic updates come with rollback + `onError` toast |
| Watched-state invalidation | `lib/invalidateWatchedState.ts` - add new watched-sensitive feeds there, nowhere else |
| Tokens in URLs | never the account JWT. Media streaming/download uses media-scoped stream tokens (mint via `useStreamToken` / `mintToken` on DownloadButton); admin streams send the Bearer via fetch-streamed SSE |
| Logic shared by two or more clients | `@evey/client` (`packages/client`) - see below |

## Shared client logic - `@evey/client`

A hook or a piece of logic that two clients need lives ONCE, in
`packages/client`, never as a copy per app. The package imports nothing from
react-native, expo or the DOM: what depends on the platform is passed in (a
logger through `setClientLogger`, the parental unlock policy through
`configureParentalUnlock`, a flag such as the phone's token-in-query image
mode) and the app wires it at startup. What does NOT belong there: screens,
UI components (React Native and the DOM share no primitives) and the three
players, which are deliberately separate implementations.

Each app keeps a one-line module at the old path (`apps/*/src/hooks/useX.ts`
re-exporting from `@evey/client`), so screens and their tests import what
they always did and a `vi.mock` of that path keeps working. New code imports
`@evey/client` directly. The tests of a shared module live next to it in the
package, once; a platform variant (the TV's unlock TTL, the phone's query
token) gets its own `*.<variant>.test.ts` there, not a copy in the app.

Adding a workspace package: `pnpm-workspace.yaml` picks it up, but the root
`vitest.config.ts` project list, each app's vitest aliases and the
`Dockerfile` (one `COPY .../package.json` line per package before
`pnpm install --frozen-lockfile`) are explicit - all three need the new name.

## i18n

Every user-facing string goes through `t()` with keys in ALL six locales
(en/fr/de/es/hi/zh). No FR/EN fallback literals in components. Reuse existing
keys before minting new ones (`common.*` is rich).

## Process

- Web changes: typecheck + vitest run inside the `evey-web` container.
- Anything touching lists, navigation or the player: verify on a 390px
  viewport (Playwright) before calling it done.
- The player is VITAL: no unattended changes to `features/playback/**`
  without running the affected flows for real.
