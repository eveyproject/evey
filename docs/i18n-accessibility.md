# Languages & Accessibility

An honest account of where Evey stands on internationalization (i18n) and
accessibility (a11y): what works, how it works, and what doesn't yet. This is a
self-hosted project built and maintained by one person - treat a11y in
particular as *best-effort, not certified*.

---

## Internationalization (i18n)

### What is and isn't translated

- **Translated:** the UI chrome - buttons, menus, settings, error messages,
  email/push notification text.
- **Not translated by Evey:** media metadata (titles, plot summaries, genres)
  comes from your metadata provider (TMDB/OMDb) in the **Metadata language** set
  in the setup wizard or in **Admin → Server Config → Language & Region**
  (`EVEY_METADATA_LANGUAGE` is only the default), which is independent of the UI
  language.
  Your own libraries, file names, and anything you type are stored as-is.

So the UI can be in French while a movie's synopsis is in English (or vice
versa) - they're two separate settings.

### Supported UI languages

Six: **English (en)**, **French (fr)**, **German (de)**, **Spanish (es)**,
**Chinese (zh)**, **Hindi (hi)**. English is the source language and the
fallback: any string missing in another language falls back to English rather
than showing a blank or a key.

### How it works

- **One source of truth:** [`/i18n/<locale>/*.yaml`](../i18n) at the repo root,
  split into namespaces (`common`, `auth`, `media`, `player`, `admin`,
  `errors`, `notifications`, `mobile`, `downloads`, `requests`). The same catalog drives
  every surface.
- The **server** loads it and serves it at `GET /api/v1/i18n/{locale}`, and uses
  it directly for translated API error messages and notification text.
- The **web, mobile, and TV apps** load translations from that endpoint
  (i18next + http-backend), cache them locally, and keep `<html lang>` in sync
  with the active language so assistive tech announces the right language.
- Language is picked from your account preference, falling back to the
  browser/device language (`Accept-Language` is parsed, region stripped:
  `fr-FR` → `fr`), then to the **Server language** of **Admin → Server Config →
  Language & Region** (which also sets the language of new accounts), then to
  English.
- Interpolation and pluralization use i18next (`{{count}}` etc.).

### Honest limitations

- **Completeness varies.** English is authoritative and always complete; the
  other five are kept at parity by hand with a check script, but a brand-new
  feature may land in English first and fall back until translated.
- **No right-to-left languages yet.** There's no Arabic/Hebrew/Persian, and the
  layout has not been built or tested for RTL - adding one is real work (CSS
  logical properties, `dir` handling), not just a translation file.
- **Translations aren't professionally reviewed.** They're community/maintainer
  quality. Corrections are welcome.
- **Locale-aware formatting is partial.** Dates/numbers rely on the platform
  `Intl`/browser locale and aren't uniformly wired on every screen.

### Adding or fixing a language

1. Copy `i18n/en/` to `i18n/<your-locale>/` and translate the YAML values
   (keep the keys).
2. Add the locale to the allowlist in
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go) (`supportedLocales`).
3. Keep the key set identical across locales - the parity check fails otherwise.

---

## Accessibility (a11y)

### Where this stands

Evey is built with semantic HTML, ARIA labels, and React Native accessibility
props, and a few things below are deliberate. But it has **not** been formally
audited against WCAG, there is **no automated accessibility testing** in the
suite, and it has **not** been systematically tested with screen readers. Parts
of it are likely usable with assistive tech; parts are likely not. If you depend
on AT, expect rough edges and please report specifics.

### What's in place

- **Reduced motion:** `prefers-reduced-motion: reduce` is honored globally -
  animations and transitions are effectively disabled when your OS asks for it.
- **Language for AT:** `<html lang>` tracks the active UI language.
- **Keyboard (player):** full keyboard controls with an on-screen shortcuts
  overlay (`?`), and shortcuts are suppressed while you're typing in a field.
- **Labelled controls:** icon-only buttons and sliders carry `aria-label` on web
  and `accessibilityLabel`/`accessibilityRole` on mobile (the larger share of
  interactive controls, not all).
- **Theming:** five themes ship in-box (default dark, AMOLED, dark-red,
  light-default, light-blue); more via theme plugins.
- **Android TV:** D-pad focus handling for remote navigation.

### Honest gaps (not done yet)

- **No screen-reader pass.** NVDA/VoiceOver/TalkBack flows are unverified;
  some custom widgets (menus, modals, the seek bar, track pickers) may not
  announce their state or role correctly.
- **Focus management isn't audited.** Focus-visible styling and focus order in
  dialogs/overlays are inconsistent in places.
- **Contrast is measured for the built-in themes only.** A unit test
  (`frontend/packages/theme/src/contrast.test.ts`) holds every shipped theme to
  WCAG AA 4.5:1 for the accent, the three text ramps and the danger set against
  the background and both surfaces; the TV app re-checks its derived tokens.
  Theme plugins and the colours a plugin injects at runtime are not checked -
  a custom theme can still fail in spots. AAA is not targeted.
- **No in-app text scaling** beyond browser/OS zoom, and no dedicated
  high-contrast mode.
- **Subtitle styling on mobile is limited** (a constraint of the underlying
  `expo-video` player), though subtitle/caption tracks themselves play.

### Reporting an issue

Tell us the page/screen, the specific control, what you expected, and the
assistive tech + version you used. Concrete reports get fixed; "it's not
accessible" can't be acted on. a11y fixes are handled best-effort.
