---
source_sha: aacd60e164be25536f5b7ca3fe0a88655d153900e00d2521c8d0795dd2769d24
reviewed: true
---
# Idiomas y accesibilidad

En qué punto está Evey en internacionalización (i18n) y accesibilidad
(a11y): qué se traduce y cómo llega el catálogo a cada cliente, y después qué
hay para las tecnologías de apoyo y qué no. El catálogo vive en
`i18n/<locale>/*.yaml`, en la raíz del repositorio; lo sirve
`internal/pkg/i18n` y lo carga el módulo i18n de cada aplicación. Es un
proyecto autoalojado que mantiene una sola persona: considera la a11y, sobre
todo, como un esfuerzo de buena voluntad, no algo certificado.

## Internacionalización (i18n)

### Qué se traduce y qué no

| Lo traduce Evey | No lo traduce Evey |
| --- | --- |
| la interfaz en sí: botones, menús, ajustes, mensajes de error, el texto de las notificaciones push, los correos de notificación (en el idioma de quien los recibe) | los metadatos de los medios (títulos, sinopsis, géneros), que vienen del proveedor de metadatos en el **Idioma de metadatos** elegido en el asistente de configuración o en **Administración → Configuración del servidor → Idioma y región** (`EVEY_METADATA_LANGUAGE` es solo el valor por defecto) |
| | tus propias bibliotecas, los nombres de archivo y todo lo que escribes, que se guarda tal cual |

Los correos de restablecimiento de contraseña se envían en inglés. El idioma
de la interfaz y el de los metadatos son dos ajustes distintos: la interfaz
puede estar en francés y la sinopsis de una película en inglés, o al revés.

### Idiomas de la interfaz

Seis: **inglés (en)**, **francés (fr)**, **alemán (de)**, **español (es)**,
**chino (zh)**, **hindi (hi)**. El inglés es el idioma de origen y el de
reserva: cualquier texto que falte en otro idioma se muestra en inglés, en
lugar de un hueco vacío o una clave.

### Cómo llega un texto a la pantalla

- **Una sola fuente de verdad:** [`/i18n/<locale>/*.yaml`](../i18n) en la
  raíz del repositorio, dividido en espacios de nombres (`common`, `auth`,
  `media`, `player`, `admin`, `errors`, `notifications`, `mobile`,
  `downloads`, `requests`). El mismo catálogo alimenta todas las superficies.
- El **servidor** lo carga y lo sirve en `GET /api/v1/i18n/{locale}`, y lo
  usa directamente para los mensajes de error traducidos de la API y el texto
  de las notificaciones.
- La **aplicación web** carga las traducciones desde ese endpoint (i18next
  con el backend HTTP), guarda una copia en el almacenamiento local del
  navegador para el siguiente arranque y mantiene `<html lang>` al día con el
  idioma activo, para que las tecnologías de apoyo anuncien el idioma
  correcto. Las **aplicaciones móvil y de TV** cargan desde el mismo endpoint
  a través del módulo i18n nativo compartido, y llevan incluida una base en
  inglés (`fallback.en.json`, `offlineStrings.ts`) para las pantallas que se
  muestran antes de que responda un servidor.
- El idioma se toma de la preferencia de tu cuenta; si no hay, del idioma del
  navegador o del dispositivo (se lee `Accept-Language` y se quita la región:
  `fr-FR` pasa a ser `fr`); después, del **Idioma del servidor** de
  **Administración → Configuración del servidor → Idioma y región** (que
  también fija el idioma de las cuentas nuevas); y por último, el inglés.
- La interpolación y los plurales usan i18next (`{{count}}`, etc.).

### Lo que todavía no está hecho

- **El nivel de completitud varía.** El inglés es la referencia y siempre
  está completo; los otros cinco se mantienen a la par a mano, con un test
  (`internal/pkg/i18n/parity_test.go`) que falla cuando el conjunto de claves
  se desajusta, pero una función nueva puede llegar primero en inglés y
  mostrarse en inglés hasta que se traduzca.
- **No hay idiomas de derecha a izquierda.** No hay árabe, hebreo ni persa, y
  la disposición no se ha construido ni probado para RTL: añadir uno es
  trabajo de verdad (propiedades lógicas de CSS, gestión de `dir`), no solo un
  archivo de traducción.
- **Las traducciones no las ha revisado un profesional.** Tienen la calidad
  que les da quien mantiene el proyecto. Las correcciones son bienvenidas.
- **El formato según el idioma es parcial.** Las fechas y los números
  dependen de `Intl` de la plataforma o del idioma del navegador, y no están
  conectados igual en todas las pantallas.

### Añadir o corregir un idioma

1. Copia `i18n/en/` en `i18n/<your-locale>/` y traduce los valores YAML
   (conserva las claves).
2. Añade el idioma a la lista permitida en
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go)
   (`supportedLocales`).
3. Mantén el mismo conjunto de claves en todos los idiomas: si no, el test de
   paridad falla, y `TestI18nPlaceholdersAgreeAcrossLocales` comprueba
   también que los `{{placeholders}}` de cada texto coinciden.

## Accesibilidad (a11y)

### En qué punto está

Evey está construido con HTML semántico, etiquetas ARIA y las propiedades de
accesibilidad de React Native, y algunas de las cosas de abajo son
deliberadas. Pero **no** se ha auditado formalmente según las WCAG, **no hay
tests automáticos de accesibilidad** en la batería de tests y **no** se ha
probado de forma sistemática con lectores de pantalla. Algunas partes
probablemente se pueden usar con tecnologías de apoyo; otras probablemente
no. Si dependes de ellas, espera asperezas y, por favor, cuéntanos los
detalles.

### Lo que ya está

| Área | Lo que está hecho |
| --- | --- |
| Movimiento reducido | `prefers-reduced-motion: reduce` se respeta en toda la aplicación (`frontend/apps/web/src/styles/main.css`): las animaciones y las transiciones quedan prácticamente desactivadas cuando tu sistema lo pide |
| Idioma para las tecnologías de apoyo | `<html lang>` sigue el idioma activo de la interfaz |
| Teclado (reproductor) | control completo con el teclado, con una capa en pantalla que muestra los atajos (`?`, `KeyboardShortcutsOverlay.tsx`); los atajos se desactivan mientras escribes en un campo |
| Controles etiquetados | los botones de solo icono y los deslizadores llevan `aria-label` en la web y `accessibilityLabel`/`accessibilityRole` en el móvil (la mayor parte de los controles interactivos, no todos) |
| Temas | se incluyen cinco temas (oscuro por defecto, AMOLED, rojo oscuro, claro por defecto, azul claro); hay más con los plugins de tema |
| Android TV | gestión del foco con la cruceta para navegar con el mando |

### Lo que todavía no está hecho

- **No se ha hecho una pasada con lector de pantalla.** Los recorridos con
  NVDA, VoiceOver o TalkBack no están verificados; algunos componentes
  propios (menús, ventanas modales, la barra de progreso, los selectores de
  pista) puede que no anuncien bien su estado o su función.
- **La gestión del foco no está auditada.** El estilo del foco visible y el
  orden del foco en diálogos y capas no son coherentes en algunos sitios.
- **El contraste solo se mide en los temas incluidos.** Un test unitario
  (`frontend/packages/theme/src/contrast.test.ts`) exige a cada tema incluido
  el nivel WCAG AA 4.5:1 para el color de acento, las tres gamas de texto y
  el juego de colores de peligro, frente al fondo y las dos superficies; la
  aplicación de TV vuelve a comprobar sus tokens derivados
  (`ThemeProvider.contrast.test.ts`). Los plugins de tema y los colores que
  un plugin inyecta mientras funciona no se comprueban: un tema propio puede
  seguir fallando en algunos puntos. El nivel AAA no es un objetivo.
- **No hay ajuste del tamaño del texto en la aplicación**, más allá del zoom
  del navegador o del sistema, ni un modo de alto contraste propio.
- **El estilo de los subtítulos en el móvil es limitado** (una restricción
  del reproductor `expo-video` que hay debajo), aunque las pistas de
  subtítulos sí se reproducen.

### Informar de un problema

Dinos la página o la pantalla, el control concreto, lo que esperabas y la
tecnología de apoyo que usaste, con su versión. Los informes concretos se
corrigen; con un «no es accesible» no se puede hacer nada. Las correcciones
de a11y se atienden en la medida de lo posible.

<!-- sources
catalogue: i18n/en, de, es, fr, hi, zh ; internal/pkg/i18n/i18n.go - supportedLocales, FallbackLocale, ParseLocale ; internal/pkg/i18n/parity_test.go ; internal/contractguard - TestI18nPlaceholdersAgreeAcrossLocales, TestServerEmittedI18nKeysExistInCatalogue
endpoint: internal/adapters/driving/http/server.go - GET /api/v1/i18n/{locale}
loaders: frontend/apps/web/src/i18n/index.ts - HttpBackend, getCached, documentElement.lang ; frontend/packages/client/src/native/i18n.ts ; frontend/apps/mobile/src/i18n/fallback.en.json ; frontend/apps/tv/src/i18n/offlineStrings.ts
mails: internal/adapters/driven/email/password_reset_emailer.go ; internal/adapters/driven/email/notification_emailer.go - WithNotificationLocale
labels: i18n admin.serverConfig.languageRegion, serverLanguage, metadataLanguage
a11y: frontend/apps/web/src/styles/main.css - prefers-reduced-motion ; frontend/apps/web/src/features/playback/KeyboardShortcutsOverlay.tsx ; frontend/packages/theme/src/themeVars.ts ; frontend/packages/theme/src/contrast.test.ts ; frontend/apps/tv/src/providers/ThemeProvider.contrast.test.ts ; frontend/apps/mobile/package.json - expo-video
-->
