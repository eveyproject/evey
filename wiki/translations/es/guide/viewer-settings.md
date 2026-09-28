---
source_sha: 5eee9b7d91ff4e7f592e19232c8e2bcaa6dbd8516301cccf7b266b23b3a06252
---
# Ajustes del espectador

Lo que cada cuenta puede cambiar por sí misma, sin pedir nada al
administrador. Casi todo está en **Ajustes** (el menú del avatar y luego
Ajustes); el resto, en el reproductor. Las aplicaciones de Android y de TV
ofrecen los mismos ajustes en su propia pantalla de Ajustes.

Los ajustes van unidos al **perfil** que está frente a la pantalla, no al
dispositivo: elija audio en inglés en la tele y el móvil también lo usará.

## Apariencia e idioma

| Ajuste | Qué hace |
| --- | --- |
| **Tema** | El tema de colores, incluidos los que haya instalado el administrador. |
| **Idioma** | El idioma de la interfaz. Hay seis: inglés, francés, alemán, español, hindi y chino. |
| **Región** | De qué país se muestran y aplican las clasificaciones de edad. **Automático** sigue al servidor. |
| **Idioma de audio preferido** | La pista que se elige primero cuando un archivo tiene varios idiomas. |
| **Idioma de subtítulos preferido** | La pista de subtítulos que se muestra cuando alguna coincide; **Sin preferencia** los deja apagados salvo que el archivo los imponga. |

Los títulos y las sinopsis siguen el idioma de metadatos del servidor, que elige
el administrador.

## Reproducción

| Ajuste | Por defecto | Qué hace |
| --- | --- | --- |
| **Reproducir automáticamente el siguiente episodio** | activado | Inicia el siguiente episodio cuando termina uno. El botón del siguiente episodio sigue disponible en cualquier caso. |
| **Bitrate máximo de streaming** | Sin límite | Un techo para sus flujos. Bájelo en una conexión lenta: el servidor convierte para caber en él en lugar de dejar que el reproductor se atasque. |
| **Bitrate máximo en datos móviles** (teléfono) | - | Un techo más bajo, aplicado solo con datos móviles. |
| **Códec de vídeo preferido** / **de audio preferido** | Auto | Déjelo en Auto salvo que un dispositivo falle con un códec: Auto deja que el servidor elija lo que el dispositivo descodifica mejor. |
| **Canales de audio máximos** | Auto | Estéreo, 5.1 o 7.1. Elija Estéreo si ve con auriculares o con los altavoces de la tele y los diálogos se oyen demasiado bajos en las mezclas envolventes. |
| **Incrustar subtítulos** | Auto | Algunos formatos de subtítulos (de imagen, o muy estilizados) no los puede dibujar cualquier reproductor; Auto solo los incrusta en la imagen en ese caso. **Incrustar siempre** y **Nunca incrustar** lo fuerzan, a costa de una conversión o de subtítulos que faltan. |
| **Permitir reproducción directa** / **stream directo** / **transcodificación** | todos activados | Las tres formas en que una película puede llegarle: tal cual, reempaquetada o convertida. Desactivar una es una herramienta de diagnóstico, no un ajuste de diario. |

El administrador puede fijar un techo más bajo para su cuenta o para las
conexiones desde fuera de casa; siempre gana el techo más bajo.

## En el reproductor

El icono del engranaje abre los ajustes del reproductor:

- **Calidad**: **Auto** se adapta a su conexión; **Original** pide el archivo
  intacto cuando el dispositivo sabe leerlo; una altura fija (1080p, 720p...)
  fuerza una conversión a ese tamaño.
- **Audio** y **Subtítulos**: cambie de pista en cualquier momento, sin volver al
  principio. **Subir subtítulo...** añade un archivo de subtítulos desde su
  ordenador para este título.
- **Sincronización de audio**: desplaza el sonido respecto a la imagen, en
  milisegundos (positivo = sonido más tarde). Disponible cuando la película se
  está convirtiendo; reinicia el flujo en la posición actual.
- **Sincronización de subtítulos**: desplaza los subtítulos (positivo = más
  tarde), al instante.
- **Estilo de subtítulos**: tamaño, color del texto, fondo, fuente, posición,
  sombra.
- **Velocidad** y **Temporizador de suspensión** (un número de minutos, o el
  final del episodio).

Alrededor de la línea de tiempo: **capítulos**, botones para **saltar la
intro**, **el resumen** y **los créditos** cuando el servidor sabe dónde están,
una cuenta atrás hasta el siguiente episodio, **Transmitir**, **Reproducir en**
(enviar a otra aplicación Evey), **Ver juntos** y **Descargar** (teléfono).

**Información del flujo** le dice cómo le llega la película - reproducción
directa, remux o transcodificación - y por qué. Es lo primero que hay que mirar
cuando algo se reproduce mal: «Códec de audio que este reproductor no puede
descodificar» o «Tasa de bits por encima del límite permitido» suelen
explicarlo.

En un ordenador, `?` muestra los atajos de teclado: `Espacio` o `K` para pausar,
`J`/`L` o las flechas izquierda y derecha para saltar, arriba y abajo para el
volumen, `F` para pantalla completa, `M` para silenciar, `C` para los
subtítulos, `<` y `>` para la velocidad, `P` para imagen en imagen, y de `0` a
`9` para saltar del 0 al 90 % de la película.

## Perfiles y control parental

**Ajustes → Perfiles** crea, edita y borra perfiles (el perfil por defecto no se
puede borrar). Cada perfil tiene:

| Campo | Qué hace |
| --- | --- |
| **Nombre** y **avatar** | Se muestran en la pantalla «¿Quién está viendo?». |
| **Perfil infantil** | Se abre sin PIN; salir de él hacia otro perfil pide el PIN de la cuenta. |
| **Clasificación de edad máxima** | Los títulos por encima nunca se envían a este perfil - ni en las listas, ni en la búsqueda, ni al reproductor. Lo decide el servidor: ninguna aplicación puede saltárselo. |
| **Idioma** y **Región** | Sustituyen, para este perfil, el idioma de la interfaz y el país de las clasificaciones. |

**Ajustes → Control parental** define el **PIN de 4 cifras** de la cuenta. La
primera vez pide la contraseña de la cuenta; para cambiarlo después, pide el PIN
actual. Un PIN es obligatorio en cuanto existe un perfil infantil.

Si el servidor no puede leer los ajustes de edad de un perfil, no muestra nada
en lugar de mostrarlo todo: un ajuste estropeado nunca abre la biblioteca a un
niño.

## Descargas (teléfono)

**Descargar** en una película, una temporada, una serie, una colección o una
lista de reproducción la guarda en el teléfono. Cuando una película es demasiado
grande para el espacio libre, la aplicación ofrece una copia más ligera
preparada por el servidor, con el tamaño esperado de cada opción. **Descargas**
lista lo que está guardado y el espacio que ocupa.

Una descarga sigue sujeta a las reglas del perfil: un título por encima del
límite de edad de un perfil no se puede reproducir desde ese perfil, aunque otro
perfil lo haya descargado. Una insignia indica cuándo una descarga ha caducado y
hay que volver a obtenerla.

## Notificaciones

**Ajustes → Notificaciones** activa las notificaciones push en este dispositivo
y fija, por separado para películas, series y música, cuánto quiere recibir:
**Todas**, **Relevantes**, **Solo importantes** o **Desactivado**. El push
requiere que el administrador lo haya configurado
([Notificaciones](../notifications.md)).

## Cuenta y dispositivos

- **Cuenta** (tarjeta del perfil): nombre de usuario, correo, contraseña (ocho
  caracteres como mínimo) y avatar. El correo es lo que permite restablecer una
  contraseña olvidada, cuando el servidor puede enviar correo.
- **Dispositivos**: los dispositivos registrados para las notificaciones push,
  que puede quitar.
- **Conexión rápida**: escriba, o escanee, el código que muestra una tele u otro
  dispositivo para conectarlo a su cuenta sin teclear su contraseña en él.
- **Historial de visualización** (menú del perfil): todo lo que ha visto, día a
  día, con un botón para quitar una entrada o borrarlo todo.
- **Complementos**: cuando el administrador ha instalado extensiones con sus
  propias preferencias, aparecen aquí.

## Solicitudes

Si el administrador lo permite, **Solicitudes de contenido** le deja buscar una
película, una serie o un artista que no está en la biblioteca y pedirlo. **Mis
solicitudes** muestra la respuesta: aceptada, rechazada o aplazada, con la nota
del administrador.
