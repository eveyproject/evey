---
source_sha: 507ac22fa3a3e19be004b58f4eb8c59130fdf70e75855b01aae6d4b67fc7e4cd
---

# Idiomas y accesibilidad

Un balance honesto de dónde está Evey en internacionalización (i18n) y
accesibilidad (a11y): qué funciona, cómo funciona y qué todavía no. Este es un
proyecto para autoalojar, escrito y mantenido por una sola persona; tómese la
accesibilidad, sobre todo, como algo que *se hace en la medida de lo posible, sin
certificación ninguna*.

---

## Internacionalización (i18n)

### Qué se traduce y qué no

- **Traducido:** la propia interfaz, es decir botones, menús, ajustes, mensajes
  de error y el texto de las notificaciones por correo y push.
- **Lo que Evey no traduce:** los metadatos de sus archivos (títulos, sinopsis,
  géneros) llegan de su proveedor de metadatos (TMDB u OMDb) en el **idioma de
  metadatos** que usted configure (`EVEY_METADATA_LANGUAGE`), que es
  independiente del idioma de la interfaz. Sus bibliotecas, sus nombres de
  archivo y todo lo que escriba usted se guardan tal cual.

De modo que la interfaz puede estar en francés mientras la sinopsis de una
película está en inglés, o al revés: son dos ajustes distintos.

### Idiomas disponibles en la interfaz

Seis: **inglés (en)**, **francés (fr)**, **alemán (de)**, **español (es)**,
**chino (zh)** e **hindi (hi)**. El inglés es el idioma de partida y el de
reserva: cualquier cadena que falte en otro idioma recae en el inglés, en lugar
de mostrar un hueco o el nombre de la clave.

### Cómo funciona

- **Una única fuente de verdad:** [`/i18n/<locale>/*.yaml`](../i18n), en la raíz
  del repositorio, repartido en espacios de nombres (`common`, `auth`, `media`,
  `player`, `admin`, `errors`, `notifications`, `mobile`, `downloads`,
  `requests`). Ese mismo catálogo alimenta todas las superficies.
- El **servidor** lo carga y lo sirve en `GET /api/v1/i18n/{locale}`, y lo usa
  también de forma directa para traducir los mensajes de error de la API y el
  texto de las notificaciones.
- Las aplicaciones **web, móvil y de televisor** cargan las traducciones desde
  ese extremo (i18next más http-backend), las guardan en caché localmente y
  mantienen `<html lang>` en sintonía con el idioma activo, para que las
  tecnologías de asistencia anuncien el idioma correcto.
- El idioma sale de la preferencia de su cuenta; si no la hay, del idioma del
  navegador o del aparato (se analiza `Accept-Language` y se descarta la región:
  `fr-FR` → `fr`); y, en última instancia, del inglés.
- La interpolación y los plurales corren a cargo de i18next (`{{count}}` y
  demás).

### Límites, dichos con franqueza

- **La cobertura no es igual en todos los idiomas.** El inglés es la referencia y
  está siempre completo; los otros cinco se mantienen a la par a mano, con un
  script de comprobación, pero una función recién llegada puede aparecer primero
  en inglés y recaer en él hasta que se traduzca.
- **Todavía no hay idiomas que se escriban de derecha a izquierda.** No hay
  árabe, ni hebreo, ni persa, y la maquetación no se ha construido ni probado
  para RTL: añadir uno es trabajo de verdad (propiedades lógicas de CSS, gestión
  de `dir`), no solo un archivo de traducción.
- **Las traducciones no las revisa ningún profesional.** Son de la calidad que
  dan la comunidad y quien mantiene el proyecto. Las correcciones son
  bienvenidas.
- **El formato según la configuración regional está a medias.** Las fechas y los
  números se apoyan en el `Intl` de la plataforma o en la configuración regional
  del navegador, y no están conectados de manera uniforme en todas las pantallas.

### Añadir o corregir un idioma

1. Copie `i18n/en/` en `i18n/<your-locale>/` y traduzca los valores del YAML
   (conservando las claves).
2. Añada la configuración regional a la lista blanca de
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go)
   (`supportedLocales`).
3. Mantenga el mismo conjunto de claves en todas las configuraciones regionales o
   la comprobación de paridad fallará.

---

## Accesibilidad (a11y)

### En qué punto estamos

Evey está construido con HTML semántico, etiquetas ARIA y las propiedades de
accesibilidad de React Native, y unas cuantas cosas de las que siguen son
deliberadas. Pero **no** se ha auditado formalmente frente a las WCAG, en la
batería de pruebas **no** hay ninguna prueba automatizada de accesibilidad y
**no** se ha probado de manera sistemática con lectores de pantalla. Es probable
que haya partes utilizables con tecnologías de asistencia, y partes que no lo
sean. Si usted depende de ellas, cuente con encontrar asperezas; le agradecemos
que nos cuente cuáles en concreto.

### Lo que ya está

- **Movimiento reducido:** se respeta `prefers-reduced-motion: reduce` en todas
  partes; cuando su sistema operativo lo pide, las animaciones y transiciones
  quedan desactivadas a efectos prácticos.
- **Idioma para las tecnologías de asistencia:** `<html lang>` sigue al idioma
  activo de la interfaz.
- **Teclado (reproductor):** control completo por teclado, con una capa en
  pantalla que enumera los atajos (`?`); los atajos se desactivan mientras se
  escribe en un campo.
- **Controles etiquetados:** los botones que solo llevan icono y los deslizadores
  llevan `aria-label` en la web y `accessibilityLabel` y `accessibilityRole` en
  el móvil (la mayor parte de los controles interactivos, no todos).
- **Temas:** cinco vienen de serie (default dark, AMOLED, dark-red,
  light-default, light-blue); hay más a través de extensiones de tema.
- **Android TV:** gestión del foco con el D-pad para navegar con el mando.

### Carencias, dichas con franqueza (aún sin hacer)

- **No se ha hecho ninguna pasada con lector de pantalla.** Los recorridos con
  NVDA, VoiceOver o TalkBack están sin verificar; algunos componentes propios
  (menús, ventanas modales, la barra de posicionamiento, los selectores de pista)
  podrían no anunciar bien su estado o su función.
- **La gestión del foco no está auditada.** El estilo de `focus-visible` y el
  orden del foco en diálogos y capas superpuestas son irregulares en algunos
  sitios.
- **El contraste está medido solo en los temas incluidos.** Una prueba unitaria
  (`frontend/packages/theme/src/contrast.test.ts`) exige a cada tema distribuido
  el 4,5:1 de WCAG AA para el acento, las tres gradaciones de texto y la gama de
  peligro, tanto contra el fondo como contra las dos superficies; la aplicación
  de televisor vuelve a comprobar sus propios tokens derivados. Ni las
  extensiones de tema ni los colores que una extensión inyecta en tiempo de
  ejecución pasan por ahí, así que un tema propio todavía puede fallar en algún
  punto. El nivel AAA no es un objetivo.
- **No hay escalado de texto dentro de la aplicación** más allá del zoom del
  navegador o del sistema, ni un modo de alto contraste dedicado.
- **El estilo de los subtítulos en el móvil es limitado** (una restricción del
  reproductor `expo-video` que hay debajo), aunque las pistas de subtítulos sí se
  reproducen.

### Informar de un problema

Díganos en qué página o pantalla, en qué control concreto, qué esperaba usted y
con qué tecnología de asistencia y qué versión. Un informe concreto se arregla;
un «no es accesible» no da nada con lo que trabajar. Las correcciones de
accesibilidad se atienden en la medida de lo posible.
