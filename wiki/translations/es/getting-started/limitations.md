---
source_sha: 6a8dc35c6cee620adb9bfbba22b808325ea49623051da7c9b0980893e4c7ff09
reviewed: true
---

# Limitaciones conocidas

Lo que Evey todavía no hace, dicho claramente, para que nadie lo descubra la
noche en que quería ver una película. Cada punto dice qué usar en su lugar.

## No hay app para iPhone, iPad ni Apple TV

Hay una app de Evey para teléfonos y tabletas Android, y otra para Android TV
y Google TV. No hay ninguna para iPhone, iPad ni Apple TV. En esos
dispositivos, abre la dirección del servidor en el navegador web.

Más: [Conectar las aplicaciones](apps.md).

## Las apps de Android todavía están en pruebas

Las apps de Android y Android TV todavía no están en Google Play para todo el
mundo. Solo las cuentas invitadas a la prueba pueden instalarlas. La app web
y la página `/tv` en el navegador de un televisor no necesitan instalación y
funcionan para todos.

Más: [Conectar las aplicaciones](apps.md).

## Ver sin conexión: solo con la app de Android

Solo la app de Android guarda una copia de una película o un episodio en el
dispositivo (**Descargas**), que se puede reproducir durante 30 días sin
contactar con el servidor. El reproductor web, la app de Android TV y la
página `/tv` reproducen desde el servidor y necesitan la conexión todo el
tiempo.

Más: [Lo que añade cada app](apps.md#what-each-app-adds).

## No hay notificaciones en el teléfono con la app cerrada

Las apps de Android y Android TV muestran sus notificaciones en su pantalla
**Notificaciones**, cuando están abiertas. Con la app cerrada no avisan ni en
el teléfono ni en el televisor. Las notificaciones que te llegan fuera de la
app vienen del navegador web (una vez activadas, en `https://`) o por correo,
cuando la persona que gestiona Evey lo ha configurado.

Más: [Dónde aparecen las notificaciones](../notifications.md#where-notifications-appear).

## Los perfiles se gestionan en la web o en el teléfono

La app de Android TV cambia de un perfil a otro y modifica algunos ajustes
del perfil en uso, pero no puede crear ni eliminar un perfil. Hazlo en la app
web o en la app de Android.

Más: [Usuarios, perfiles y control parental](../administration/users.md#profiles-kids-profiles-and-the-pin).

## Transmitir: desde Chrome o la app de Android

El botón **Transmitir** está en el reproductor web en Chrome y en la app de
Android. Firefox no lo tiene, ni la app de Android TV, porque ya es el
televisor. Safari, y Chrome en un teléfono Android, usan la reproducción
remota del propio navegador, que funciona de otra manera.

Más: [Enviar a Chromecast](../playback/casting.md).

## Evey nunca se actualiza solo

El servidor comprueba si existe una versión más reciente y lo indica en el
panel de administración, pero no instala nada por su cuenta: la persona que
lo gestiona actualiza cuando le viene bien, con dos comandos. Las apps de
Android muestran un aviso cuando son más antiguas que el servidor.

Más: [Actualizar Evey](../backups.md#updating-evey) y
[Mantener las apps al día](apps.md#keeping-the-apps-up-to-date).
