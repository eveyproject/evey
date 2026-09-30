---
source_sha: 73f4232ebdb512554747cf529508f5987b0fb6a98e909a18fad120a3acb05c1c
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
| **Bitrate máximo** | Un límite al bitrate que reciben los reproductores de la cuenta (0 = ilimitado). Un título por encima se transcodifica hasta ese límite. |
| **Perfiles máx** | Cuántos perfiles puede crear la cuenta (0 = ilimitado). |
| **Dispositivos máx por perfil** | Cuántos dispositivos puede registrar cada perfil (0 = ilimitado). |

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
