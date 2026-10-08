# Languages & Accessibility

Where Evey stands on internationalization (i18n) and accessibility (a11y):
what is translated and how the catalogue reaches each client, then what is
in place for assistive technology and what is not. The catalogue lives in
`i18n/<locale>/*.yaml` at the repository root, served by
`internal/pkg/i18n` and loaded by each app's i18n module. This is a
self-hosted project maintained by one person: treat a11y in particular as
best-effort, not certified.

## Internationalization (i18n)

### What is and is not translated

| Translated by Evey | Not translated by Evey |
| --- | --- |
| the UI chrome: buttons, menus, settings, error messages, push notification text, notification e-mails (in the recipient's language) | media metadata (titles, plot summaries, genres), which comes from the metadata provider in the **Metadata language** set in the setup wizard or in **Admin → Server Config → Language & Region** (`EVEY_METADATA_LANGUAGE` is only the default) |
| | your own libraries, file names, and anything you type, stored as-is |

Password reset e-mails are sent in English. The UI language and the
metadata language are two separate settings: the UI can be in French while a
film's synopsis is in English, or the reverse.

### Supported UI languages

Six: **English (en)**, **French (fr)**, **German (de)**, **Spanish (es)**,
**Chinese (zh)**, **Hindi (hi)**. English is the source language and the
fallback: any string missing in another language falls back to English rather
than showing a blank or a key.

### How a string reaches the screen

- **One source of truth:** [`/i18n/<locale>/*.yaml`](../i18n) at the repo
  root, split into namespaces (`common`, `auth`, `media`, `player`, `admin`,
  `errors`, `notifications`, `mobile`, `downloads`, `requests`). The same
  catalogue drives every surface.
- The **server** loads it and serves it at `GET /api/v1/i18n/{locale}`, and
  uses it directly for translated API error messages and notification text.
- The **web app** loads translations from that endpoint (i18next with the
  HTTP backend), keeps a copy in the browser's local storage for the next
  start, and keeps `<html lang>` in sync with the active language so
  assistive tech announces the right language. The **mobile and TV apps**
  load from the same endpoint through the shared native i18n module and
  ship a bundled English floor (`fallback.en.json`, `offlineStrings.ts`)
  for the screens shown before a server answers.
- Language is picked from your account preference, falling back to the
  browser/device language (`Accept-Language` is parsed, region stripped:
  `fr-FR` becomes `fr`), then to the **Server language** of **Admin → Server
  Config → Language & Region** (which also sets the language of new
  accounts), then to English.
- Interpolation and pluralization use i18next (`{{count}}` etc.).

### What is not done yet

- **Completeness varies.** English is authoritative and always complete; the
  other five are kept at parity by hand, with a test
  (`internal/pkg/i18n/parity_test.go`) that fails when a key set drifts,
  but a brand-new feature may land in English first and fall back until
  translated.
- **No right-to-left languages.** There is no Arabic/Hebrew/Persian, and the
  layout has not been built or tested for RTL: adding one is real work (CSS
  logical properties, `dir` handling), not just a translation file.
- **Translations are not professionally reviewed.** They are maintainer
  quality. Corrections are welcome.
- **Locale-aware formatting is partial.** Dates and numbers rely on the
  platform `Intl`/browser locale and are not uniformly wired on every screen.

### Adding or fixing a language

1. Copy `i18n/en/` to `i18n/<your-locale>/` and translate the YAML values
   (keep the keys).
2. Add the locale to the allowlist in
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go)
   (`supportedLocales`).
3. Keep the key set identical across locales: the parity test fails
   otherwise, and `TestI18nPlaceholdersAgreeAcrossLocales` checks that the
   `{{placeholders}}` of each string agree too.

## Accessibility (a11y)

### Where this stands

Evey is built with semantic HTML, ARIA labels and React Native accessibility
props, and a few things below are deliberate. But it has **not** been
formally audited against WCAG, there is **no automated accessibility
testing** in the suite, and it has **not** been systematically tested with
screen readers. Parts of it are likely usable with assistive tech; parts are
likely not. If you depend on AT, expect rough edges and please report
specifics.

### What is in place

| Area | What is done |
| --- | --- |
| Reduced motion | `prefers-reduced-motion: reduce` is honored globally (`frontend/apps/web/src/styles/main.css`): animations and transitions are effectively disabled when your OS asks for it |
| Language for AT | `<html lang>` tracks the active UI language |
| Keyboard (player) | full keyboard controls with an on-screen shortcuts overlay (`?`, `KeyboardShortcutsOverlay.tsx`); shortcuts are suppressed while you type in a field |
| Labelled controls | icon-only buttons and sliders carry `aria-label` on web and `accessibilityLabel`/`accessibilityRole` on mobile (the larger share of interactive controls, not all) |
| Theming | five themes ship in-box (default dark, AMOLED, dark-red, light-default, light-blue); more via theme plugins |
| Android TV | D-pad focus handling for remote navigation |

### What is not done yet

- **No screen-reader pass.** NVDA/VoiceOver/TalkBack flows are unverified;
  some custom widgets (menus, modals, the seek bar, track pickers) may not
  announce their state or role correctly.
- **Focus management is not audited.** Focus-visible styling and focus order
  in dialogs and overlays are inconsistent in places.
- **Contrast is measured for the built-in themes only.** A unit test
  (`frontend/packages/theme/src/contrast.test.ts`) holds every shipped theme
  to WCAG AA 4.5:1 for the accent, the three text ramps and the danger set
  against the background and both surfaces; the TV app re-checks its derived
  tokens (`ThemeProvider.contrast.test.ts`). Theme plugins and the colours a
  plugin injects at runtime are not checked: a custom theme can still fail
  in spots. AAA is not targeted.
- **No in-app text scaling** beyond browser/OS zoom, and no dedicated
  high-contrast mode.
- **Subtitle styling on mobile is limited** (a constraint of the underlying
  `expo-video` player), though subtitle/caption tracks themselves play.

### Reporting an issue

Tell us the page/screen, the specific control, what you expected, and the
assistive tech + version you used. Concrete reports get fixed; "it's not
accessible" cannot be acted on. a11y fixes are handled best-effort.

<!-- sources
catalogue: i18n/en, de, es, fr, hi, zh ; internal/pkg/i18n/i18n.go - supportedLocales, FallbackLocale, ParseLocale ; internal/pkg/i18n/parity_test.go ; internal/contractguard - TestI18nPlaceholdersAgreeAcrossLocales, TestServerEmittedI18nKeysExistInCatalogue
endpoint: internal/adapters/driving/http/server.go - GET /api/v1/i18n/{locale}
loaders: frontend/apps/web/src/i18n/index.ts - HttpBackend, getCached, documentElement.lang ; frontend/packages/client/src/native/i18n.ts ; frontend/apps/mobile/src/i18n/fallback.en.json ; frontend/apps/tv/src/i18n/offlineStrings.ts
mails: internal/adapters/driven/email/password_reset_emailer.go ; internal/adapters/driven/email/notification_emailer.go - WithNotificationLocale
labels: i18n admin.serverConfig.languageRegion, serverLanguage, metadataLanguage
a11y: frontend/apps/web/src/styles/main.css - prefers-reduced-motion ; frontend/apps/web/src/features/playback/KeyboardShortcutsOverlay.tsx ; frontend/packages/theme/src/themeVars.ts ; frontend/packages/theme/src/contrast.test.ts ; frontend/apps/tv/src/providers/ThemeProvider.contrast.test.ts ; frontend/apps/mobile/package.json - expo-video
-->
