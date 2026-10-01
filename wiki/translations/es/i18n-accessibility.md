---
source_sha: 6ad73949864d3c9b0dea20d620fb0f5e4a2b176cba0f54c8e15090f66f2b929c
reviewed: false
---
# Idiomas y accesibilidad

Un balance honesto de dónde está Evey en internacionalización (i18n) y
accesibilidad (a11y): qué funciona, cómo funciona y qué todavía no. Es un
proyecto autoalojado creado y mantenido por una sola persona: considera la
accesibilidad en particular como *un esfuerzo razonable, no certificado*.

---

## Internacionalización (i18n)

### Qué se traduce y qué no

- **Traducido:** la interfaz (botones, menús, ajustes, mensajes de error, texto
  de las notificaciones por correo y push).
- **No lo traduce Evey:** los metadatos de los medios (títulos, sinopsis, géneros)
  vienen de tu proveedor de metadatos (TMDB/OMDb) en el **Idioma de metadatos**
  definido en el asistente de configuración o en **Admin → Configuración del
  servidor → Idioma y región** (`EVEY_METADATA_LANGUAGE` es solo el valor por
  defecto), que es independiente del idioma de la interfaz.
  Tus propias bibliotecas, los nombres de archivo y todo lo que escribes se guardan
  tal cual.

Así, la interfaz puede estar en francés mientras la sinopsis de una película está
en inglés (o al revés): son dos ajustes distintos.

### Idiomas de la interfaz

Seis: **inglés (en)**, **francés (fr)**, **alemán (de)**, **español (es)**,
**chino (zh)**, **hindi (hi)**. El inglés es el idioma de origen y el de
respaldo: cualquier cadena que falte en otro idioma vuelve al inglés en lugar de
mostrar un hueco o una clave.

### Cómo funciona

- **Una sola fuente de verdad:** [`/i18n/<locale>/*.yaml`](../i18n) en la raíz del
  repositorio, dividido en espacios de nombres (`common`, `auth`, `media`, `player`, `admin`,
  `errors`, `notifications`, `mobile`, `downloads`, `requests`). El mismo catálogo
  alimenta todas las superficies.
- El **servidor** lo carga y lo sirve en `GET /api/v1/i18n/{locale}`, y lo usa
  directamente para los mensajes de error traducidos de la API y el texto de las
  notificaciones.
- Las **aplicaciones web, móvil y de televisor** cargan las traducciones desde ese
  endpoint (i18next + http-backend), las guardan en caché localmente y mantienen
  `<html lang>` sincronizado con el idioma activo para que las tecnologías de
  asistencia anuncien el idioma correcto.
- El idioma se elige a partir de la preferencia de tu cuenta, si no del idioma del
  navegador o del dispositivo (se analiza `Accept-Language`, sin la región:
  `fr-FR` → `fr`), si no del **Idioma del servidor** de **Admin → Configuración del
  servidor → Idioma y región** (que fija también el idioma de las cuentas nuevas),
  y si no del inglés.
- La interpolación y los plurales usan i18next (`{{count}}`, etc.).

### Límites, sin rodeos

- **La completitud varía.** El inglés es la referencia y siempre está completo;
  los otros cinco se mantienen a la par a mano con un script de comprobación, pero
  una función nueva puede llegar primero en inglés y mostrarse en inglés hasta que
  se traduzca.
- **Todavía no hay idiomas de derecha a izquierda.** No hay árabe, hebreo ni
  persa, y la disposición no se ha construido ni probado para RTL: añadir uno es
  trabajo real (propiedades lógicas de CSS, gestión de `dir`), no solo un archivo
  de traducción.
- **Las traducciones no tienen revisión profesional.** Son de calidad de la
  comunidad o del mantenedor. Las correcciones son bienvenidas.
- **El formato según el idioma es parcial.** Las fechas y los números dependen de
  `Intl` o del idioma del navegador y no están conectados de forma uniforme en
  todas las pantallas.

### Añadir o corregir un idioma

1. Copia `i18n/en/` a `i18n/<your-locale>/` y traduce los valores YAML
   (conserva las claves).
2. Añade el idioma a la lista de permitidos en
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go) (`supportedLocales`).
3. Mantén el mismo conjunto de claves en todos los idiomas: si no, la comprobación
   de paridad falla.

---

## Accesibilidad (a11y)

### Dónde está

Evey se construye con HTML semántico, etiquetas ARIA y las propiedades de
accesibilidad de React Native, y algunas cosas de abajo son deliberadas. Pero
**no** se ha auditado formalmente según las WCAG, **no hay pruebas automáticas de
accesibilidad** en la batería de pruebas y **no** se ha probado de forma
sistemática con lectores de pantalla. Partes de Evey probablemente se pueden usar
con tecnologías de asistencia; otras probablemente no. Si dependes de ellas,
espera asperezas y, por favor, informa de casos concretos.

### Lo que hay

- **Movimiento reducido:** `prefers-reduced-motion: reduce` se respeta en todas
  partes: las animaciones y transiciones quedan prácticamente desactivadas cuando
  tu sistema lo pide.
- **Idioma para las tecnologías de asistencia:** `<html lang>` sigue el idioma
  activo de la interfaz.
- **Teclado (reproductor):** controles completos por teclado con una superposición
  de atajos en pantalla (`?`), y los atajos se desactivan mientras escribes en un
  campo.
- **Controles etiquetados:** los botones y deslizadores con solo un icono llevan
  `aria-label` en la web y `accessibilityLabel`/`accessibilityRole` en el móvil
  (la mayor parte de los controles interactivos, no todos).
- **Temas:** se incluyen cinco temas (oscuro por defecto, AMOLED, rojo oscuro,
  claro por defecto, azul claro); más mediante plugins de tema.
- **Android TV:** gestión del foco con el mando direccional para navegar con el
  mando a distancia.

### Carencias, sin rodeos (todavía no hecho)

- **Ninguna pasada con lector de pantalla.** Los recorridos con
  NVDA/VoiceOver/TalkBack no están verificados; algunos widgets personalizados
  (menús, ventanas modales, la barra de búsqueda, los selectores de pistas) pueden
  no anunciar correctamente su estado o su función.
- **La gestión del foco no está auditada.** El estilo del foco visible y el orden
  del foco en diálogos y superposiciones no son coherentes en algunos sitios.
- **El contraste solo se mide para los temas integrados.** Una prueba unitaria
  (`frontend/packages/theme/src/contrast.test.ts`) exige a cada tema incluido el
  nivel WCAG AA 4.5:1 para el acento, las tres gamas de texto y el conjunto de
  peligro frente al fondo y las dos superficies; la aplicación de televisor vuelve
  a comprobar sus tokens derivados. Los plugins de tema y los colores que un plugin
  inyecta en ejecución no se comprueban: un tema personalizado puede fallar en
  algunos puntos. No se apunta a AAA.
- **Sin escalado de texto en la aplicación** más allá del zoom del navegador o del
  sistema, y sin modo de alto contraste dedicado.
- **El estilo de los subtítulos en el móvil es limitado** (una restricción del
  reproductor `expo-video` subyacente), aunque las pistas de subtítulos se
  reproducen.

### Informar de un problema

Dinos la página o pantalla, el control concreto, lo que esperabas y la tecnología
de asistencia que usaste, con su versión. Los informes concretos se corrigen; "no
es accesible" no permite actuar. Las correcciones de accesibilidad se tratan como
un esfuerzo razonable.
