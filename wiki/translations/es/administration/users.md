---
source_sha: 93a7bbd14075f9a561be863d85aabddbc09ed2371d0b43390f14adc88a26778f
reviewed: false
---
# Usuarios, perfiles y control parental

## Cuentas y perfiles

Una **cuenta** inicia sesión: tiene un nombre de usuario y una contraseña (o una
identidad de inicio de sesión único, consulta [OIDC](../oidc.md)), y es
administradora o no. Un **perfil** ve contenido: cada cuenta tiene uno o varios
perfiles, cada uno con su propio historial, puntos de reanudación, favoritos,
idiomas y página de inicio. Un hogar suele compartir una cuenta, con un perfil
por persona.

## Crear cuentas

**Admin → Usuarios** ofrece dos formas:

<figure class="evey-shot" markdown="span">
  ![Admin → Usuarios en el servidor de demostración](../images/admin-users.webp){ loading=lazy }
  <figcaption><strong>Admin → Usuarios</strong>: cada cuenta con sus perfiles, su rol y su estado (interfaz en inglés).</figcaption>
</figure>

- **Crear usuario**: tú eliges el nombre de usuario y la contraseña.
- **Crear enlace de invitación**: la persona elige la suya. Un enlace puede tener
  fecha de caducidad y un número máximo de usos, restringir las bibliotecas que
  ve la nueva cuenta, limitar el número de perfiles que puede crear y crear un
  administrador. **Enlaces de invitación** los lista; un enlace se puede revocar.

## Lo que un administrador puede restringir por cuenta

| Ajuste | Efecto |
| --- | --- |
| **Acceso a bibliotecas** | Las bibliotecas que ve la cuenta. Todo lo demás está ausente de sus búsquedas, su página de inicio y las respuestas de la API. |
| **Bitrate máximo** | Un límite al bitrate del vídeo que el servidor codifica para los reproductores de la cuenta (0 = ilimitado). |
| **Perfiles máx** | Cuántos perfiles puede crear la cuenta (0 = ilimitado). |
| **Dispositivos máx por perfil** | Cuántas reproducciones puede tener en curso a la vez cada perfil, sumando sus dispositivos (0 = ilimitado). Una más se rechaza al empezar. |

**Conectarse como** abre el servidor tal como lo ve ese usuario, para comprobar
una restricción o reproducir un problema. Un aviso permanece visible, y
**Salir** te devuelve a tu propia cuenta.

## Perfiles infantiles y el PIN

Un perfil marcado como **Perfil infantil** se abre sin PIN. Salir de él hacia un
perfil que no es infantil pide el **PIN de 4 cifras**, definido en **Ajustes →
Control parental**. Definir el PIN por primera vez pide la contraseña de la
cuenta, y mientras exista un perfil infantil la cuenta debe tener un PIN.

La gestión de perfiles y del control parental solo es posible desde un perfil que
no sea infantil.

## Límites de edad

Cada perfil puede tener una **Clasificación de edad máxima** (**Ajustes →
Perfiles → Editar**). El sistema de clasificación es el del país definido en
**Admin → Configuración del servidor → Idioma y región**.

El límite lo aplica el servidor, no las aplicaciones: un título por encima no se
oculta en la interfaz, simplemente nunca se envía. Esto vale para explorar y
buscar, para la reproducción y para las descargas. Si el servidor no puede leer
los ajustes de un perfil (por un error de base de datos, por ejemplo), rechaza el
contenido en lugar de permitirlo. Los administradores no están sujetos a los
límites de los perfiles.

Una película descargada en un teléfono sigue sujeta al límite sin conexión: un
perfil infantil no puede abrir una descarga hecha por otro perfil por encima de
su límite de edad.

## Solicitudes

Los usuarios pueden buscar una película, una serie o un artista que falte en las
bibliotecas y pedirlo. Las solicitudes llegan a **Admin → Solicitudes de
contenido**, donde un administrador las aprueba, las rechaza o las aplaza, con
una nota. Quién puede solicitar se define ahí: todo el mundo por defecto, con
excepciones por usuario.

## Enlaces de descarga públicos

En la página de una película o un episodio, un administrador puede crear un
**Enlace de descarga público** (aplicaciones web y Android). Es una dirección
que cualquiera puede abrir, sin cuenta, para descargar el archivo original; se
copia al portapapeles.

- Caduca a las 48 horas. Mediante la API del servidor puede durar hasta 72
  horas y limitarse a un número de descargas (`POST /api/v1/media/{id}/share`).
- Solo los administradores pueden crear, listar y revocar enlaces. Las
  aplicaciones solo los crean: listar y revocar se hace mediante la API
  (`GET /api/v1/admin/share-links`, `DELETE /api/v1/share-links/{id}`).
- Con el acceso remoto desactivado, el enlace solo funciona desde las redes
  locales.
- Quien tenga el enlace puede descargar el archivo hasta que caduque:
  compártelo como el propio archivo.

## La página de inicio

La página de inicio es una pila de filas; una fila sin nada que mostrar se
oculta.

| Fila | Qué muestra |
| --- | --- |
| **Reanudar reproducción** | Lo que el perfil empezó y no terminó, lo más reciente primero. Un título cuenta como visto al 90 %. |
| **A continuación** | Para cada serie de la que el perfil ha visto un episodio, el siguiente episodio sin ver. |
| **Colecciones** | Las colecciones, con un enlace a la lista completa. |
| **Recomendado para ti** | Películas y series de los géneros habituales del perfil, la mejor coincidencia primero y luego por valoración. Un género cuenta cuando aparece en al menos 3 de las últimas 200 reproducciones del perfil. |
| **Más popular** | Las películas y series más reproducidas en el servidor en los últimos 90 días. |
| Una fila por biblioteca | Las últimas incorporaciones a esa biblioteca, la más reciente primero. |

Recomendado para ti y Más popular dejan fuera lo que el perfil ya ha terminado;
una serie cuenta como terminada cuando se han visto todos sus episodios. Los
plugins pueden añadir sus propias filas.

**Admin → Configuración del servidor → Feeds de inicio** fija el valor por
defecto para todos los perfiles: qué filas aparecen, en qué orden y cuántos
elementos muestra cada una. Cada perfil ordena después su propio inicio con
**Personalizar** (en la página de inicio de las aplicaciones web y móvil, en la
cabecera de la aplicación de TV): mostrar u ocultar una fila, moverla o
**Restablecer**. La elección se guarda en el servidor y sigue al perfil en cada
dispositivo. Un perfil puede mostrar una fila que el administrador ha ocultado,
salvo la fila de un plugin: ahí la elección del administrador es definitiva.
