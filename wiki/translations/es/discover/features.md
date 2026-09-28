---
source_sha: 51c7a248181b2b8e94e8f646b4cf91f36a740e2bc6410ad95e7d53bb1099c0cd
---
# Lo que Evey le ofrece

Un recorrido por Evey desde el lado de quienes lo usan cada noche. Cada sección
termina con el lugar donde están los ajustes correspondientes.

## Una biblioteca que se cuida sola

Deje archivos en las carpetas que Evey vigila y aparecen solos, normalmente en
cuestión de segundos: el servidor detecta los archivos nuevos, reconoce
películas y episodios por su nombre y trae carteles, fondos, sinopsis, reparto,
equipo y valoraciones en el idioma que usted eligió. Las películas de una misma
saga se agrupan en colecciones, las series en temporadas, la música en artistas
y álbumes, las fotos en álbumes.

Cuando un nombre es demasiado ambiguo para reconocerlo, el título se conserva en
lugar de perderse, y un administrador puede asociarlo a mano o corregir sus
datos. Lo que usted cambia a mano se queda cambiado: la siguiente actualización
no lo sobrescribe.

Cuatro tipos de biblioteca: **películas**, **series**, **música** y **fotos**.

Ajustes: [Primeros pasos](../guide/first-steps.md#add-your-libraries) ·
[Administración](../guide/admin-settings.md#media).

## Una reproducción que simplemente arranca

Usted pulsa Reproducir; Evey decide cómo. Si el dispositivo sabe leer el archivo
tal cual, el archivo se envía intacto, en su calidad original. Si solo el
contenedor es el problema, se reempaqueta al vuelo sin tocar la imagen. Si el
dispositivo de verdad no sabe descodificarlo - un códec que desconoce, HDR en
una pantalla que no lo muestra, una conexión demasiado fina para el original -
Evey convierte solo lo necesario, con la tarjeta gráfica cuando el servidor
tiene una.

Mientras mira:

- **reanudar en cualquier parte** - pare en la tele y siga en el móvil en el
  mismo segundo;
- **saltar la intro, el resumen y los créditos** con un botón, gracias a bases
  de datos comunitarias o a la propia detección del servidor;
- **el siguiente episodio** empieza solo, con una cuenta atrás que puede
  cancelar;
- **miniaturas en la línea de tiempo** muestran adónde va cuando arrastra la
  barra;
- **las pistas de audio y subtítulos** siguen sus preferencias de idioma, se
  cambian a mitad de película sin volver a empezar, y los subtítulos se pueden
  reestilizar, desplazar en el tiempo o añadir desde un archivo;
- **capítulos**, un **temporizador de suspensión**, la **velocidad** de
  reproducción y atajos de teclado en el navegador.

Ajustes: [Ajustes del espectador](../guide/viewer-settings.md#in-the-player) ·
[Transcodificación y hardware](../transcoding.md).

## Algo para cada uno en casa

Cada cuenta contiene varios **perfiles**, cada uno con su historial, sus puntos
de reanudación, su idioma, sus favoritos y su página de inicio. Elegir el perfil
abre la aplicación, como en una tele familiar.

- Los **perfiles infantiles** se abren sin trabas; salir de ellos hacia un
  perfil adulto pide un PIN de cuatro cifras.
- Los **límites de edad** se fijan por perfil y los aplica el servidor: un
  título por encima del límite no lo oculta la aplicación, nunca se envía. Si el
  servidor no puede leer el límite de un perfil, no muestra nada en lugar de
  mostrarlo todo.
- **Invitaciones**: un administrador envía un enlace, la persona elige una
  contraseña y llega con las bibliotecas y el número de perfiles decididos para
  ella.
- **Solicitudes**: un usuario puede buscar una película, una serie o un artista
  que falta y pedirlo; el administrador acepta, rechaza o aplaza, con una nota.

Ajustes: [Ajustes del espectador](../guide/viewer-settings.md#profiles-and-parental-control) ·
[Administración](../guide/admin-settings.md#users-and-access).

## Ver juntos, desde casas distintas

**Ver juntos** abre una sala alrededor de una película, un episodio o una cola
de música. Comparta el código de seis caracteres o el enlace; todos los que se
unen reproducen al ritmo del anfitrión - pausa, saltos y reanudación incluidos.
Quien se haya quedado atrás puede ponerse al día por su cuenta sin molestar a
la sala.

## Música y fotos

La música tiene artistas, álbumes, listas de reproducción, reproducción
aleatoria, una cola, reproducción sin pausas para los álbumes que la necesitan,
y un minirreproductor que sigue sonando mientras navega. Las fotos tienen
álbumes, un visor a pantalla completa con los datos de la toma, y un pase de
diapositivas.

## Sus propias estanterías

- **Favoritos** y **valoraciones** por perfil.
- **Colecciones**: las sagas que Evey construye solo, más las suyas, en el orden
  que quiera.
- **Colecciones inteligentes**: un filtro guardado (género, año, resolución,
  idioma...) que se llena solo a medida que crece la biblioteca.
- **Listas de reproducción** de películas, episodios y canciones, en orden o
  aleatorias.
- **Historial** y **estadísticas** de lo que ha visto, que puede borrar.
- Una **página de inicio** que puede reordenar: seguir viendo, a continuación,
  añadidos recientemente, recomendaciones, colecciones.

## Sin conexión, en el móvil

La aplicación Android descarga una película, una temporada, una serie entera,
una colección o una lista de reproducción para el avión. Cuando una película es
demasiado grande para el móvil, ofrece una copia más ligera preparada por el
servidor. Las descargas respetan el perfil que las hizo: un perfil infantil no
puede abrir una película que otro perfil descargó por encima de su límite de
edad.

## Vigilar el servidor

Los administradores ven en directo quién mira qué y cómo - reproducido tal cual,
reempaquetado o convertido, y por qué - junto con la carga del servidor, su
capacidad de conversión medida, la calidad de streaming por dispositivo y una
página de salud de los medios que encuentra y repara los archivos en los que se
salta mal. La base de datos se respalda antes de cada actualización, y según una
programación si usted fija una.

Ajustes: [Administración](../guide/admin-settings.md#monitoring-and-maintenance).

## Ampliarlo

Las extensiones añaden fuentes de metadatos, notificaciones, webhooks, widgets
para la página de inicio y temas. Se ejecutan en un entorno aislado con solo los
permisos que declaran, y un administrador ve esos permisos antes de instalar. Un
tema es una hoja de estilo: cambia el aspecto de la interfaz sin tocar su
código.

Más información: [Extensiones](../plugins.md) · [Referencia de la API de temas](../themes/api-reference.md).

## Probarlo

La demo pública en [demo.eveyproject.org](https://demo.eveyproject.org) es el mismo servidor que
instalaría, con una pequeña biblioteca de películas con licencia libre. Inicie
sesión con `demo` / `demo`. Explore, reproduzca, cree colecciones, abra la
administración: los ajustes se muestran pero la mayoría están bloqueados, y la
instancia se restablece con regularidad.
