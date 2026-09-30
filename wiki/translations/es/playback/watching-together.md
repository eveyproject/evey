---
source_sha: a6742845a66194948de961b0490bc70821d9e1a06b1c6ff4aaed96f02c9910ba
reviewed: false
---
# Ver juntos

**Ver juntos** mantiene a varias personas en el mismo título en el mismo momento,
cada una en su propio dispositivo: una sala guarda la posición y todos los
reproductores la siguen. El flujo de cada espectador se sigue decidiendo para su
propio dispositivo, como en
[Cómo elige Evey el modo de reproducción](how-playback-is-decided.md): una
persona puede estar en Reproducción directa mientras otra se transcodifica.

## Crear una sala y unirse

| | Web | Android | Android TV |
| --- | --- | --- | --- |
| Crear una sala | **Ver juntos** en el reproductor | **Ver juntos** en el reproductor | - |
| Unirse con una cuenta | El enlace de la sala, o el código en **Escuchar juntos** → **Unirse a una sala** | El código en **Escuchar juntos** | El código en **Ajustes** → **Ver juntos** |
| Unirse sin cuenta | **Ver juntos** en la página de inicio de sesión, con el código | Igual | - |

La persona que crea la sala es su anfitrión y ve su código: seis caracteres,
letras y cifras. Los códigos se escriben sin preocuparse por mayúsculas,
espacios ni por confundir `O` con `0` e `I` o `L` con `1`.

Unirse con una cuenta sigue las reglas habituales: la biblioteca debe estar
compartida con esa cuenta y el control parental del perfil debe permitir el
título. Un perfil restringido tampoco puede abrir una sala sobre un título que no
puede ver.

## Invitados sin cuenta

Un invitado que introduce un código en la página de inicio de sesión ve ese
título y nada más:

- solo streaming, nunca descarga;
- solo salas de vídeo, no salas de música;
- el acceso dura mientras se usa, caduca tras tres horas sin reproducción y tras
  24 horas en cualquier caso, y termina en cuanto se cierra la sala.

El código es lo único que protege una sala, así que compártelo solo con las
personas que quieras dentro. Los códigos erróneos están limitados por dirección y
en todo el servidor, lo que deja fuera de alcance adivinarlos.

## Qué se mantiene sincronizado

- **En Android y Android TV**, reproducir, pausar y saltar del anfitrión mueve a
  toda la sala. Los controles de los demás miembros no afectan a nadie más.
- **En el reproductor web**, pausar y saltar se quedan en tu propia pantalla.
  **Ponerme al día** te devuelve adonde está la sala, sin mover a nadie más.
  **Sincronizar a todos** lleva a toda la sala a tu posición; cualquier miembro
  puede usarlo, no solo el anfitrión.

Quien se une tarde empieza donde está la sala ahora, no donde estaba cuando se
envió la última orden.

Si el anfitrión se va, el miembro que lleva más tiempo en la sala pasa a ser el
anfitrión. Cuando se va el último miembro, la sala se cierra y se retira el
acceso de invitado que daba su código. Una sala en la que nunca entró nadie se
elimina al cabo de una hora.

## Escuchar juntos

Una sala de música se crea desde **Escuchar juntos** con una cola de álbumes,
pistas y listas de reproducción. El anfitrión reproduce, pausa y avanza por la
cola; todos los miembros le siguen. Los invitados sin cuenta no pueden unirse a
las salas de música.
