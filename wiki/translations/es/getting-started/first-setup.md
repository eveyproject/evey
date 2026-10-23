---
source_sha: 48225cad93d795d54f36c6e4db0c828700357d5e317bec3f404927fba2c1cdf4
reviewed: true
---
# Primera configuración { #first-setup }

Evey está funcionando y has abierto su dirección en un navegador, por
ejemplo `http://192.168.1.20:9091` (si no sabes qué dirección es:
[paso 3 de la instalación](../installation.md#the-address-to-open-next)).

Quedan cuatro pasos antes de que en casa se pueda ver algo:

1. [El asistente de configuración](#1-the-setup-wizard): más o menos un
   minuto.
2. [Añadir tus bibliotecas](#2-add-your-libraries): decirle a Evey dónde
   están tus archivos.
3. [Invitar a las personas de tu casa](#3-invite-your-household): una cuenta por
   persona, o una sola cuenta compartida con un perfil por persona.
4. [Conectar las aplicaciones](#4-connect-the-apps): teléfonos, tablets y
   televisores.

## 1. El asistente de configuración { #1-the-setup-wizard }

Mientras el servidor no tenga administrador, su dirección siempre abre el
asistente. Arriba muestra **Paso 1 de 4 - Idioma**, y cuatro puntos que se
van rellenando a medida que avanzas.

### Paso 1 de 4 - Idioma { #step-1-of-4---language }

La tarjeta dice **Elige el idioma del servidor**. Elige:

- **Idioma**: el idioma por defecto de los perfiles nuevos, y de los títulos
  y resúmenes que Evey descarga. El asistente cambia a ese idioma en el
  momento.
- **Región**: se rellena según el idioma. Decide qué clasificaciones por
  edad se muestran. También puedes elegir **Automático**.

Puedes cambiar las dos cosas más adelante en el panel de administración.
Pulsa **Continuar**.

### Paso 2 de 4 - Bienvenida { #step-2-of-4---welcome }

No hay nada que rellenar. Pulsa **Comenzar**.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![Paso 1 del asistente: idioma y región](../images/setup-wizard-1-language.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Paso 1: el idioma del servidor y la región que se usa para las clasificaciones por edad (interfaz en inglés).</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Paso 2 del asistente: bienvenida](../images/setup-wizard-2-welcome.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Paso 2: nada que rellenar.</figcaption>
</figure>

</div>

### Paso 3 de 4 - Cuenta de administrador { #step-3-of-4---admin-account }

La tarjeta dice **Crear cuenta de administrador**. Es tu propia cuenta, la
que gestiona el servidor.

| Campo | Qué escribir |
| --- | --- |
| **Nombre de usuario** | Al menos 3 caracteres. |
| **Contraseña** | Al menos 8 caracteres. |
| **Confirmar contraseña** | La misma contraseña otra vez. |
| **Nombre del servidor** | Opcional. Un nombre como «Salón». Se ve en la página de inicio de sesión, en la pestaña del navegador, en la lista de servidores de las apps y en los correos. Puedes cambiarlo más adelante en **Administración → Configuración del servidor → Nombre del servidor**. |

Pulsa **Continuar**. Evey crea la cuenta y te deja con la sesión iniciada.
El asistente no volverá a aparecer.

### Paso 4 de 4 - Primera biblioteca { #step-4-of-4---first-library }

La tarjeta dice **Añade tu primera biblioteca**. Una biblioteca es un tipo de
contenido guardado en una o varias carpetas, por ejemplo «Películas».

- **Nombre de la biblioteca**: por ejemplo «Películas».
- **Tipo de biblioteca**: **Películas**, **Series**, **Música** o **Fotos**.
- **Rutas**: elige una carpeta de la lista. Muestra las carpetas que Evey
  puede ver, empezando por `/media`. Para escribir una ruta como
  `/media/movies`, elige **Introducir ruta personalizada...**.

Pulsa **Crear**: Evey empieza a leer la carpeta. También puedes pulsar
**Omitir** y añadir las bibliotecas más tarde. En los dos casos llegas a la
página de inicio.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![Paso 3 del asistente: la cuenta de administrador](../images/setup-wizard-3-admin.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Paso 3: la cuenta de administrador; el nombre del servidor es opcional.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Paso 4 del asistente: la primera biblioteca](../images/setup-wizard-4-library.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Paso 4: un nombre, un tipo y una carpeta tal como la ve Evey.</figcaption>
</figure>

</div>

??? note "Si el asistente no aparece, o aparece dos veces"

    En cuanto existe un administrador, la dirección abre la página de inicio
    de sesión, y `/setup` también te lleva allí. Si dos personas abren el
    asistente a la vez, solo la primera se queda con la cuenta de
    administrador: a la segunda se la envía a la página de inicio de sesión.

<figure class="evey-shot" markdown="span">
  ![El panel de administración del servidor de demostración](../images/admin-dashboard.webp){ loading=lazy }
  <figcaption>El panel de administración una vez iniciada la sesión: el panel principal y todas las secciones en el menú lateral (interfaz en inglés).</figcaption>
</figure>

## 2. Añadir tus bibliotecas { #2-add-your-libraries }

Ve a **Administración → Bibliotecas → Añadir biblioteca**. Pon un nombre, un
tipo (Películas, Series, Música o Fotos) y una o varias carpetas.

Las carpetas se nombran como las ve Evey, no como están en tu ordenador. Con
el archivo compose de la [instalación](../installation.md#the-compose-file),
tu carpeta de archivos es `/media`. Así que una carpeta `/srv/media/films` de
tu ordenador es `/media/films` en Evey.

Evey empieza a leer la carpeta por su cuenta. Los pósteres, los resúmenes y
el reparto llegan en segundo plano, así que una biblioteca grande se va
llenando en unos minutos. Los archivos nuevos que añadas más tarde aparecen
solos.

¿Tus títulos no aparecen, o tienen el póster equivocado? Consulta
[Bibliotecas y metadatos](../administration/libraries.md): cómo nombrar los
archivos y cómo corregir un título mal reconocido.

## 3. Invitar a las personas de tu casa { #3-invite-your-household }

Hay dos formas de darle una cuenta a alguien, las dos en
**Administración → Usuarios**.

**Enviar un enlace de invitación** (la persona elige su propia contraseña):

1. Abre el panel de administración desde la dirección que usarán los demás,
   por ejemplo `http://192.168.1.20:9091`, y no `http://localhost:9091`. El
   enlace se crea a partir de la dirección de tu navegador.
2. Pulsa **Crear enlace de invitación**. Puedes limitar cuánto tiempo
   funciona (**Fecha de expiración (opcional)**), cuántas veces se puede usar
   (**Máximo de usos (vacío = ilimitado)**) y qué bibliotecas ve la cuenta
   nueva (**Bibliotecas accesibles**).
3. En **Enlaces de invitación**, pulsa **Copiar enlace** y envíalo por
   mensaje o por correo.
4. La persona abre el enlace y ve **¡Has sido invitado! Completa tus datos a
   continuación.** Elige un nombre de usuario y una contraseña (al menos 8
   caracteres), y ya tiene la sesión iniciada.

**O crear tú la cuenta**: pulsa **Crear usuario**, elige el **Nombre de
usuario** y la **Contraseña**, y dáselos a la persona.

Cada cuenta empieza con un perfil, que lleva el nombre de usuario. Una casa
puede compartir una sola cuenta, con un perfil por persona: añádelos desde
**Ajustes → Perfiles → Crear perfil**. Para un niño, marca **Perfil
infantil** y elige una **Clasificación de edad máxima**. A partir de ahí,
todos los demás perfiles de esa cuenta piden un **PIN (4 dígitos)**, para
que el niño no pueda pasar a ellos.

Después, envía tres cosas a cada persona: la dirección, su nombre de usuario
y su contraseña (o el enlace de invitación), y la página escrita para ella,
[Ver con Evey](watching.md).

Más opciones (límites por cuenta, horarios permitidos, PIN, límites de
edad):
[Usuarios, perfiles y control parental](../administration/users.md).

## 4. Conectar las aplicaciones { #4-connect-the-apps }

- **En un navegador** no hay nada que instalar: es la dirección que ya has
  abierto.
- **En un teléfono, una tablet o un televisor Android**, la app pide primero
  la dirección del servidor. Escribe la **misma dirección que en el
  navegador**, con `http://` y el puerto, por ejemplo
  `http://192.168.1.20:9091`. Después inicia sesión.
- **En un televisor**, puedes iniciar sesión con un código que apruebas desde
  tu teléfono, en lugar de escribir una contraseña con el mando.

Paso a paso, y qué significa cada mensaje de error:
[Conectar las aplicaciones](apps.md#the-server-address-to-enter).

## Lo que el servidor hace solo { #what-the-server-does-by-itself }

No tienes que configurar nada de esto:

- **Cómo reproduce cada dispositivo un archivo.** Cada dispositivo le dice al
  servidor qué puede reproducir. Para cada título, el servidor envía el
  archivo tal cual, lo cambia a otro formato de archivo, o convierte solo lo
  que el dispositivo no puede reproducir. Consulta
  [Cómo elige Evey el modo de reproducción](../playback/how-playback-is-decided.md).
- **A cuántas personas puede atender.** En su primer momento tranquilo, el
  servidor mide cuánto vídeo puede convertir a la vez, con el procesador y
  con la tarjeta gráfica si la hay, y nunca acepta más. Tarda unos minutos.
  Mientras tanto, una reproducción que necesita una conversión tiene que
  esperar. Consulta
  [Transcodificación y capacidad del servidor](../playback/transcoding.md).
- **El acceso desde fuera de casa.** Cerrado hasta que le des al servidor una
  dirección pública. Consulta
  [Acceso remoto y HTTPS](../administration/remote-access.md).
- **Una copia de seguridad de la base de datos** antes de cada actualización
  que la modifica. Para tener copias periódicas basta con un ajuste:
  [Copias de seguridad y actualizaciones](../backups.md).

## Antes de compartir la dirección fuera de casa { #before-you-share-the-address-outside-your-home }

1. Pon el servidor detrás de HTTPS. Evey solo usa HTTP simple, que sirve en
   casa pero no por internet.
   [Acceso remoto y HTTPS](../administration/remote-access.md).
2. Programa copias de seguridad y guarda una copia de la carpeta `evey-data`.
   Contiene la base de datos y la clave que protege todos los inicios de
   sesión.
   [Copias de seguridad y actualizaciones](../backups.md).
