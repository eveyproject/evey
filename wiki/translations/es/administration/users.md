---
source_sha: a4932bd9b417f0f18be610734a595c186387167b23522b0dbd34267c0fc17fc7
reviewed: true
---
# Usuarios, perfiles y control parental { #users-profiles-and-parental-control }

En esta página importan dos palabras:

- Una **cuenta** es lo que inicia sesión: un nombre de usuario y una
  contraseña.
- Un **perfil** es una persona que ve contenido. Cada perfil tiene su propio
  historial, sus puntos de reanudación, sus favoritos, sus idiomas y su
  página de inicio.

Una cuenta puede tener varios perfiles. La mayoría de los hogares comparten
una cuenta, con un perfil por persona. También puedes darle a cada adulto su
propia cuenta.

Esta página empieza por las tareas habituales: añadir a alguien, añadir a un
niño. Los controles más finos (límites por cuenta, horario de uso,
solicitudes, la página de inicio, enlaces de descarga) vienen después.

## Cuentas y perfiles { #accounts-and-profiles }

Una cuenta tiene un nombre de usuario y una contraseña, o inicia sesión con
inicio de sesión único (ver [OIDC](../oidc.md)). Es una cuenta de
administrador o una cuenta normal. Cada cuenta nueva empieza con un perfil,
que lleva el nombre de su usuario.

## Crear una cuenta { #create-an-account }

<figure class="evey-shot" markdown="span">
  ![Administración → Usuarios en el servidor de demostración](../images/admin-users.webp){ loading=lazy }
  <figcaption><strong>Administración → Usuarios</strong>: cada cuenta con sus perfiles, su rol y su estado (interfaz en inglés).</figcaption>
</figure>

Ve a **Administración → Usuarios**. Hay dos formas.

**Enviar un enlace de invitación**, para que la persona elija su propia
contraseña:

1. Pulsa **Crear enlace de invitación**.
2. Si quieres, limítalo: **Fecha de expiración (opcional)**, **Máximo de
   usos (vacío = ilimitado)**, **Bibliotecas accesibles** (las bibliotecas
   que verá la nueva cuenta) y el número de perfiles que puede crear. Marca
   **Crear como administrador** solo para alguien que vaya a gestionar el
   servidor contigo.
3. En **Enlaces de invitación**, pulsa **Copiar enlace** y envíalo.

El enlace se construye con la dirección de tu navegador. Abre el panel de
administración desde la dirección que usarán los demás (por ejemplo
`http://192.168.1.20:9091`), no desde `localhost`. **Revocar** hace que un
enlace deje de funcionar.

**O crear tú mismo la cuenta**: pulsa **Crear usuario**, elige el **Nombre
de usuario** y la **Contraseña**, y dáselos a la persona. Marca
**Administrador** para un administrador. También puedes fijar en ese momento
las bibliotecas, el número de perfiles y el número de dispositivos (ver
[Lo que un administrador puede limitar en cada cuenta](#what-an-administrator-can-restrict-per-account)).

En la fila de una cuenta:

- **Desactivar** bloquea sus inicios de sesión sin borrar nada. **Activar**
  la deja entrar de nuevo.
- **Eliminar** borra la cuenta y sus perfiles.

## Perfiles, perfiles infantiles y el PIN { #profiles-kids-profiles-and-the-pin }

**Para añadir un perfil**: desde esa cuenta, ve a **Ajustes → Perfiles →
Crear perfil**. Un administrador también puede añadir uno a cualquier
cuenta, desde **Editar usuario → Nuevo perfil**. Un perfil tiene un
**Nombre**, un **Idioma** y una **Región**.

**Para añadir el perfil de un niño**:

1. Crea el perfil y marca **Perfil infantil**.
2. Elige una **Clasificación de edad máxima**. Si dejas **Por defecto
   (12+)**, el límite es 12.
3. Pon un **PIN (4 dígitos)** a todos los demás perfiles de la cuenta. El
   servidor lo exige en cuanto existe un perfil infantil, y no guarda un
   perfil que no lo tenga.

Lo que ve entonces el niño:

- El perfil infantil se abre sin PIN.
- Para pasar a otro perfil, se pide el PIN de ese perfil.
- Los títulos por encima del límite de edad no aparecen nunca (ver
  [Límites de edad](#age-limits)).

??? info "Detalles del PIN y quién puede gestionar los perfiles"

    - Poner un PIN por primera vez pide la contraseña de la cuenta.
      Cambiarlo pide el PIN actual. **Ajustes → Control parental** fija el
      PIN del perfil que estás usando.
    - Los perfiles y los ajustes parentales solo se pueden gestionar desde un
      perfil que no sea infantil.
    - La cuenta de un administrador no puede tener un perfil infantil: el
      límite ocultaría parte de la propia biblioteca del administrador.

## Límites de edad { #age-limits }

Es el servidor, no la app, quien aplica el límite de edad. Un título por
encima del límite no solo se oculta en pantalla: nunca se envía al perfil
infantil. Esto vale para navegar, buscar, reproducir y descargar.

- **Qué clasificaciones se usan.** La edad se lee del sistema de
  clasificación de la **Región** del perfil (**Ajustes → Perfiles →
  Editar**). Con **Automático**, la región sale del idioma del perfil, y se
  usa el sistema británico cuando eso no basta.
- **Un título sin una clasificación utilizable** se oculta al perfil
  infantil en lugar de mostrarse.
- **En caso de duda, no.** Si el servidor no puede leer los ajustes de un
  perfil (por ejemplo, por un error de la base de datos), rechaza el
  contenido en lugar de permitirlo.
- **También las descargas.** Una película descargada en un teléfono
  conserva su límite sin conexión: un perfil infantil no puede abrir una
  descarga hecha por otro perfil si supera su límite de edad.

Los límites de los perfiles no se aplican a los administradores.

## Lo que un administrador puede limitar en cada cuenta { #what-an-administrator-can-restrict-per-account }

Estos ajustes están en **Crear usuario** y **Editar usuario**, y en la fila
de la cuenta en **Administración → Usuarios**.

| Ajuste | Qué hace |
| --- | --- |
| **Acceso a bibliotecas** (el botón **Bibliotecas** de la fila) | Las bibliotecas que ve la cuenta. Las demás no le aparecen en ningún sitio: ni en la búsqueda ni en su página de inicio. Un administrador siempre lo ve todo. |
| **Bitrate máximo** (el botón **Bitrate**) | Limita la calidad de vídeo que recibe la cuenta, uno de seis valores predefinidos de 1 a 40 Mbps, o Sin límite. Un título que lo supera se convierte para bajar a ese valor. |
| **Máximo de perfiles** | Cuántos perfiles puede crear la cuenta (0 = sin límite). |
| **Máximo de dispositivos por perfil** | Cuántos vídeos puede reproducir cada perfil a la vez, sumando sus dispositivos (0 = sin límite). Uno más se rechaza al empezar. |

**Conectarse como** muestra el servidor exactamente como lo ve ese usuario.
Úsalo para comprobar una restricción o para ver un problema con tus propios
ojos. Un aviso se queda en pantalla, y **Salir** te devuelve a tu propia
cuenta.

## Horario de uso { #opening-hours }

Puedes dejar que un perfil infantil vea contenido solo a ciertas horas.

1. En **Ajustes → Perfiles**, pulsa el icono del reloj junto al perfil
   infantil.
2. Activa **Limitar al horario de uso**. Evey propone de lunes a viernes de
   18:00 a 22:00, y sábado y domingo de 9:00 a 12:00 y de 14:00 a 21:00.
3. Ajusta las horas, con **Semana / fin de semana** (un bloque de lunes a
   viernes y otro para el fin de semana) o con **Día por día**.

Un día puede tener varias franjas horarias. Una franja puede pasar de
medianoche (de 20:00 a 0:30). Un día sin ninguna franja está cerrado.

Lo que pasa:

- **Fuera del horario**, el perfil está cerrado en todas partes: navegar,
  películas, música, descargas. Todas las apps muestran una pantalla de
  bloqueo que dice cuándo vuelve a abrirse.
- **Diez minutos antes del cierre**, el reproductor muestra un aviso. A la
  hora de cierre, la reproducción se detiene.
- **Para dejar entrar al niño de todos modos**, escribe el **PIN parental**
  en la pantalla de bloqueo: el PIN de un perfil de la misma cuenta que no
  sea infantil. Abre el perfil infantil hasta su siguiente apertura. El PIN
  del propio niño no sirve ahí. **Cambiar de perfil** vuelve a la lista de
  perfiles.

??? info "Zona horaria y casos especiales"

    - El servidor aplica el horario en su propia zona horaria:
      **Administración → Configuración del servidor → Idioma y región → Zona
      horaria del servidor** (si está vacía, la variable `TZ`, y si no,
      Europe/Paris). Cambiar la hora de una tablet no cambia nada.
    - No se puede volver de **Día por día** a **Semana / fin de semana**
      cuando los días de un bloque son distintos, para que no se pierda nada
      sin querer.
    - Un horario que el servidor no puede leer mantiene el perfil cerrado.
    - Una película descargada que se reproduce sin conexión nunca llega al
      servidor, así que el horario no se le aplica.

## Solicitudes { #requests }

Los demás pueden pedir una película, una serie o un artista que todavía no
tienes. Sus solicitudes llegan a **Administración → Solicitudes de
contenido**. Para cada una, elige **Aprobar**, **Rechazar** o **Más tarde**,
y añade una nota si quieres. La persona recibe un aviso con tu respuesta.

Para elegir quién puede pedir, abre la pestaña **Acceso**: **Permitir
solicitudes por defecto para todos los usuarios**, y después **Excepciones
por usuario** para los casos especiales.

## La página de inicio { #the-home-page }

La página de inicio está formada por filas. Una fila que no tiene nada que
mostrar se oculta.

| Fila | Qué muestra |
| --- | --- |
| **Reanudar reproducción** | Lo que el perfil empezó y no terminó, lo más reciente primero. Un título cuenta como visto al 90 %. |
| **A continuación** | Para cada serie que el perfil ha empezado, el siguiente episodio que todavía no ha visto. |
| **Colecciones** | Las colecciones, con un enlace a la lista completa. |
| **Recomendado para ti** | Películas y series de los géneros que más ve el perfil, primero las que mejor encajan y después por valoración. Un género cuenta cuando aparece en al menos 3 de las últimas 200 reproducciones del perfil. |
| **Más populares** | Las películas y series más reproducidas en el servidor en los últimos 90 días. |
| Una fila por biblioteca | Lo último que se ha añadido a esa biblioteca, lo más nuevo primero. |

**Recomendado para ti** y **Más populares** dejan fuera lo que el perfil ya ha
terminado. Una serie cuenta como terminada cuando se han visto todos sus
episodios. Los plugins pueden añadir sus propias filas.

**Quién decide las filas:**

- Tú fijas lo que tiene cada perfil por defecto en **Administración →
  Configuración del servidor → Feeds de inicio**: qué filas aparecen, en qué
  orden y cuántos elementos muestra cada una.
- Después, cada perfil puede organizar su propia página de inicio con
  **Personalizar** (en la página de inicio de la web y de las apps del
  teléfono, en la barra superior de la app de TV): mostrar u ocultar una
  fila, moverla, o **Restablecer**. Esto se guarda en el servidor y se aplica
  al perfil en todos sus dispositivos.
- Un perfil puede mostrar una fila que tú has ocultado, salvo la fila de un
  plugin: ahí, tu decisión es definitiva.

## Enlaces de descarga públicos { #public-download-links }

En la página de una película o de un episodio, un administrador puede crear
un **Enlace de descarga público** (web y app de Android). Cualquiera que
tenga el enlace puede descargar el archivo original, sin cuenta. El enlace
se copia para que lo pegues.

- Deja de funcionar a las 48 horas.
- Con el acceso remoto desactivado, solo funciona desde la red de tu casa.
- Quien tenga el enlace puede descargar el archivo hasta que caduque:
  compártelo con el mismo cuidado que el propio archivo.

??? info "A través de la API del servidor"

    Solo los administradores pueden crear, listar y revocar enlaces. Las
    apps solo los crean; para listarlos y revocarlos hay que pasar por la
    API (`GET /api/v1/admin/share-links`, `DELETE /api/v1/share-links/{id}`).
    Con la API, un enlace puede durar hasta 72 horas y limitarse a un número
    de descargas (`POST /api/v1/media/{id}/share`).

<!-- sources
create user / edit user: frontend/apps/web/src/features/admin/CreateUserForm.tsx, EditUserModal.tsx (CreateProfileDialog), UserRow.tsx (edit, impersonate, libraries, bitrate, enable/disable, delete) ; i18n admin.users.createUser, username, password, admin, libraryAccessTitle, maxProfiles, maxDevicesPerProfile, impersonate, exitImpersonation ; common.enable, disable, delete
invites: frontend/apps/web/src/features/admin/CreateInviteForm.tsx, InviteLinkRow.tsx ; i18n admin.users.maxUsesLabel, expiresAtLabel, inviteLibraries, inviteMaxProfilesLabel, inviteIsAdmin, inviteLinks, copyLink, revoke ; internal/app/user/access_service.go (RegisterWithInvite, RevokeInviteLink)
bitrate: frontend/apps/web/src/features/admin/users-constants.ts BITRATE_OPTIONS (1-40 Mbps) ; internal/app/user/bitrate_limit.go
max devices per profile = concurrent active playback sessions of the profile: internal/app/playback/command_service.go (DeviceQuotaExceeded, ErrMaxDevicesPerProfile)
profiles: frontend/apps/web/src/features/profile/EditProfileDialog.tsx (isKids, maxAge kids-only, pin, region, locale), CreateProfileDialog.tsx, ProfileManagerList.tsx (Clock on kids profiles) ; i18n auth.profile.manage.createProfile, fields.*, settings.parental.*, settings.parental.defaultMaxAge ; errors.auth.pin_required_for_non_kids, parental_needs_non_kids
kids invariants: internal/app/user/user_service.go checkProfileInvariants (ErrKidsRequiresPIN, ErrAdminCannotBeKids, ErrMaxProfilesReached) ; service.go ErrPINRequired ; internal/domain/user/model.go EffectiveParental, DefaultKidsMaxAge = 12
rating region: internal/pkg/region/region.go Resolve (profile region, then locale, then GB ; server region passed empty) ; internal/adapters/driving/http/parental.go BlockedByParental (WorstCaseMinAge, worst < 0 blocked) ; grpcapi/library_handler_parental.go
server-side limit, fail closed, admins exempt: internal/adapters/driving/http/parental.go checkParental ; grpcapi/library_handler_parental.go
downloads: internal/adapters/driving/http/compat_download_handler.go (checkParental) ; frontend/apps/mobile/src/lib/offlineAccess.ts
library access: internal/app/user/access_service.go (GrantMany, RevokeAllLibraryAccess) ; UserRow hides the Libraries button for admins
requests: i18n admin.mediaRequests.* (approve, reject, later, tabAccess, accessDefaultLabel, accessOverridesTitle) ; requests.*
share links: internal/adapters/driving/grpcapi/interceptors/rbac.go (ShareService RoleAdmin) ; grpcapi/share_handler.go (48 h default) ; internal/app/media/share_service.go (max 72 h, maxViews) ; http/share_download_handler.go (no auth) ; cmd/evey/gateway.go - networkGateExempt (share not exempt) ; frontend/apps/web/src/features/library/ShareLinkButton.tsx, mobile media/[id].tsx (isAdmin) ; proto/evey/v1/share.proto
opening hours: internal/domain/user/schedule.go - AccessSchedule, DefaultKidsSchedule, Window, AccessAt, OverrideUntil ; internal/app/user/profile_schedule.go - Unlock (adult PIN only) ; internal/adapters/driving/http/profile_closed.go (httpAuthMiddleware, httpStreamAuthMiddleware) ; grpcapi/interceptors/profile_hours.go ; grpcapi/playback_closing_soon.go (closingWarnLead 10 min) ; internal/app/config/timezone.go - KeyServerTimezone ; postgres profile_repo.go - scheduleFromJSON (unreadable = closed) ; i18n auth.profile.schedule.*, auth.profile.locked.*, admin.serverConfig.timezone
home: internal/app/feed/* ; i18n admin.serverConfig.feedConfig.*, media.home.*
first profile named after the username: internal/app/user/user_service.go, access_service.go - RegisterWithInvite (NewProfile with u.Username)
invite link built from the admin's browser address: frontend/apps/web/src/features/admin/InviteLinkRow.tsx - inviteUrl (window.location.origin)
requester notified of the answer: internal/app/mediarequest/service.go - Answer ; i18n notifications push.request_answered
labels: i18n auth.profile.schedule.enable, modeWeek, modeDaily ; auth.profile.locked.pinLabel, switchProfile ; media.shareLink ; admin.users.admin, libraries, bitrate, inviteIsAdmin, revoke
-->
