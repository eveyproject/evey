---
source_sha: 65686e994e8b0eed0c73e145729f00f455caa301b21644be8c6b23c67d7964d3
---
# Por qué Evey

Las películas ya las tiene. Ripeó los discos, guardó los archivos, ordenó las
carpetas. Lo que falta es lo que hace agradable verlas: una pared de carteles,
el episodio en el que se quedó, la pista de audio correcta, la misma biblioteca
en la tele, el móvil y el portátil, y una forma de compartirla con las personas
con las que vive sin entregársela a una empresa.

Evey es esa parte. Se ejecuta en su máquina - un NAS, un mini PC, un portátil
viejo - y sirve sus propios archivos a sus propias pantallas. Cinco compromisos
guían cada decisión, y cuando dos de ellos chocan, este es el orden en que se
imponen.

## 1. Seguro para ponerlo en Internet

Un servidor multimedia solo es útil si se puede llegar a él desde el sofá, desde
el tren y desde casa de los padres. Por eso Evey está hecho para ser accesible,
no escondido, y sus valores por defecto dan por hecho que la red es hostil:

- el acceso remoto se abre cuando usted publica el servidor en una dirección
  pública, y se queda cerrado en caso contrario - siempre puede decidirlo
  explícitamente;
- cuando el servidor no puede comprobar quién es usted (una base de datos que
  no responde, un ajuste parental que no puede leer), rechaza en lugar de dejar
  pasar la petición;
- las extensiones solo obtienen los permisos que su autor declaró, y el núcleo
  las confina;
- el servidor nunca se ejecuta como administrador del sistema.

## 2. Instalable en pocas líneas

Un archivo compose, un contenedor, ninguna contraseña que inventar. La base de
datos y el motor de búsqueda viven dentro del contenedor y generan sus propios
secretos la primera vez que arranca. Todo lo que quiera cambiar después -
idiomas, límites de conversión, tarjeta gráfica, programaciones - está en el
panel de administración, no en un archivo de configuración que haya que editar
y luego reiniciar.

## 3. Ampliable en el lenguaje que usted escriba

Evey se amplía con extensiones: fuentes de metadatos, widgets de la página de
inicio, temas. Una extensión es un programa que el servidor lanza y con el que
habla por una conexión local, así que puede escribirse en el lenguaje que ya
conoce - hay kits listos para siete de ellos. Como las extensiones se ejecutan
en un entorno aislado en lugar de recibir confianza ciega, instalar la de otra
persona es algo razonable.

## 4. Rápido, y honesto al respecto

La forma más rápida de reproducir un archivo es enviarlo tal cual. Evey lo hace
en cuanto el dispositivo sabe leerlo, convierte solo la parte que el
dispositivo no maneja (a menudo solo el sonido), y decide una vez por
reproducción en lugar de cambiar de idea a mitad de película. Las cifras detrás
de esas decisiones se miden con archivos reales y máquinas reales, no se
suponen - una optimización que parecía ingeniosa solo se mantuvo después de
demostrar que daba la misma respuesta que la herramienta a la que sustituía.

## 5. Para muchos espectadores a la vez, en hardware modesto

El objetivo es un hogar y sus amigos en un NAS, no un centro de datos. Por eso
Evey mide lo que su máquina es capaz de hacer la primera vez que arranca, admite
solo tantas conversiones simultáneas como puede soportar, pausa su trabajo en
segundo plano mientras alguien mira, y se dimensiona según el hardware que
encuentra. Una máquina lenta reproduce menos flujos; no los reproduce todos mal.

---

Dos cosas que Evey deliberadamente no es. No es un catálogo de todos los tipos
de medios resueltos a medias - la reproducción es su oficio. Y no es un producto
que le pida que confíe en él: lo que hace el servidor, y lo que aún no sabe
impedir, está escrito en el [modelo de seguridad](SECURITY.md) en lugar de
darse por sobreentendido.

**Siguiente:** [lo que Evey le ofrece](discover/features.md) ·
[los dispositivos en los que funciona](discover/apps.md) ·
[instalarlo](installation.md)
