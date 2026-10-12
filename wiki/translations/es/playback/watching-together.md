---
source_sha: 9fad7f52fd0f1b44bc2f65ef902487f1f2929c2f19b89072be7f363968ba8bb8
reviewed: true
---
# Ver juntos

Ve la misma película al mismo tiempo que otras personas, cada una en su
pantalla. Una persona abre una sala y las demás se unen con un código. No hay
nada que configurar en el servidor.

En resumen:

1. Pon la película en el reproductor web o en la app de Android y pulsa
   **Ver juntos**. Eres el anfitrión de la sala y ves su código: seis letras
   y números.
2. Envía el código a las personas que quieres que te acompañen.
3. Se unen con el código: desde su propia cuenta (consulta la tabla de
   abajo) o, sin cuenta, con **Ver juntos** en la página de inicio de sesión.

Quién puede pausar o mover la película para todos depende de la app:
consulta [Qué controla cada miembro](#what-each-member-controls).

Cada persona sigue recibiendo la versión de la película adecuada para su
propio dispositivo, como se explica en
[Cómo elige Evey el modo de reproducción](how-playback-is-decided.md).

## Crear una sala y unirse

| | Web | Android | Android TV |
| --- | --- | --- | --- |
| Crear una sala | **Ver juntos** en el reproductor | **Ver juntos** en el reproductor | - |
| Unirse con una cuenta | El enlace de la sala, o el código en **Escuchar juntos** → **Unirse a una sala** | El código en **Escuchar juntos** | El código en **Ajustes** → **Ver juntos** |
| Unirse sin cuenta | **Ver juntos** en la página de inicio de sesión, con el código | Igual | - |

Al escribir el código no hace falta ser exacto: da igual usar mayúsculas o
no, poner espacios, o escribir `O` en lugar de `0`, o `I` o `L` en lugar de
`1`.

Unirse con una cuenta sigue las reglas de siempre: la biblioteca tiene que
estar compartida con esa cuenta, y los ajustes de control parental del perfil
tienen que permitir el título. Un perfil con restricciones tampoco puede abrir
una sala con un título que no tiene permiso para ver.

## Qué controla cada miembro

| | Anfitrión | Otros miembros |
| --- | --- | --- |
| Android | Reproducir, pausar y saltar a otro momento afectan a toda la sala. | Reproducir, pausar y saltar a otro momento solo afectan a tu pantalla. |
| Android TV | Reproducir y pausar afectan a toda la sala; un salto solo afecta al televisor. | Igual que en Android. |
| Web | Reproducir, pausar y saltar a otro momento solo afectan a tu pantalla, también para el anfitrión. | Igual. |

En el reproductor web, unos botones concretos sustituyen al control del
anfitrión. **Sincronizar a todos** lleva a toda la sala a tu posición y
reanuda la reproducción; lo puede usar cualquier miembro, no solo el
anfitrión. **Ponerme al día** te lleva a donde está la sala, sin mover a
nadie más; aparece en la página de invitado (para quien se une con un código,
sin cuenta), no en el reproductor con la sesión iniciada.

Quien se une tarde empieza donde está la sala en ese momento, no donde
estaba cuando se envió la última orden.

Si el anfitrión se va, el miembro que lleva más tiempo en la sala pasa a ser
el anfitrión. Cuando se va el último miembro, la sala se cierra y se retira
el acceso de invitado que daba su código. Una sala en la que nunca entró
nadie se borra al cabo de una hora.

## Invitados sin cuenta

Un invitado que escribe un código en la página de inicio de sesión ve ese
título y nada más:

- solo en streaming, nunca descargando;
- solo salas de vídeo, no salas de música;
- el acceso dura mientras se usa, caduca tras tres horas sin reproducción y,
  en cualquier caso, tras 24 horas, y termina en cuanto se cierra la sala.

!!! warning
    El código es lo único que protege una sala, así que compártelo solo con
    las personas que quieres en ella. Los intentos con códigos erróneos están
    limitados por dirección y en todo el servidor, así que no se pueden
    adivinar probando.

## Escuchar juntos

Una sala de música se crea desde **Escuchar juntos**, con una cola de
álbumes, canciones y listas de reproducción. El anfitrión reproduce, pausa y
avanza por la cola; todos los miembros le siguen. Los invitados sin cuenta no
pueden unirse a las salas de música.
