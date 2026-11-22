---
source_sha: bc4e8000ca3b6f13e4b5d57a81bf41b5b314d18f1578b3eb3a7b6737c5d1b091
reviewed: true
---
# Actualizaciones de las apps servidas por la instancia { #app-updates-served-by-the-instance }

Las apps para teléfono y para TV cargan su JavaScript desde la instancia de
Evey a la que están conectadas. Una corrección del JavaScript de la app llega
así a un dispositivo sin una publicación en la tienda, en cuanto se actualiza
su servidor. Los cambios nativos siguen necesitando una.

Una app solo avanza: toma una versión más reciente que la que ejecuta y
conserva su propio código frente a un servidor más antiguo.

## Cómo encajan las piezas { #how-it-fits-together }

- **La imagen lleva los bundles.** `make client-updates` construye un bundle
  firmado por app en `client-updates/`, y la imagen copia ese directorio a
  `/app/client-updates` (`EVEY_CLIENT_UPDATES_DIR`). Una imagen construida sin
  ese paso no sirve ninguna actualización, y las apps conservan el código con
  el que se construyeron.
- **El servidor solo entrega bytes.**
  `GET /api/v1/client-updates/{app}/manifest` y
  `.../assets/{platform}/{runtime}/{name}` hablan el protocolo Expo Updates
  (`internal/adapters/driving/http/client_update_handler.go`). La instancia no
  guarda ninguna clave, no construye ningún manifiesto y solo abre archivos
  dentro de ese directorio.
- **Las apps verifican.** Cada bundle está firmado por el proyecto y se envía
  con el certificado de la clave que lo firmó; las apps comprueban ese
  certificado contra la raíz compilada en ellas,
  `frontend/client-updates/certificate.pem`. Una instancia solo puede
  distribuir código publicado; todo lo demás se rechaza en el dispositivo.
- **Las apps nunca retroceden.** Consulta «Suelo de versión» más abajo.
- **Las apps siguen al servidor activo.** Una vez elegido un servidor, la app
  apunta expo-updates hacia él y descarga su bundle en segundo plano
  (`frontend/packages/client/src/native/clientUpdates.ts`). El bundle arranca
  en el siguiente inicio. El teléfono ofrece un reinicio cuando es una versión
  de la app distinta de la que está en marcha, nunca encima del reproductor; la
  TV nunca, porque recargar en caliente es demasiado pesado para un aparato pequeño. Sin
  servidor elegido, la app no pregunta a nadie.

## Versión del runtime nativo { #native-runtime-version }

Un bundle solo funciona en la compilación nativa para la que se hizo. Cada app
declara una `runtimeVersion` en su `app.json` (`mobile-1`, `tv-1`), y un
bundle se almacena y se solicita con ese nombre.

**Sube `runtimeVersion` en el mismo cambio que cualquier cosa nativa**: un
módulo nativo nuevo o actualizado, una actualización del SDK de Expo o de
React Native, un plugin de configuración, el manifiesto de Android o el código
Go enlazado en `offline-audio`. La app para teléfono mantiene su proyecto
Android en git, así que el valor también vive en
`frontend/apps/mobile/android/app/src/main/res/values/strings.xml`.

Una instancia no tiene ningún bundle para un runtime con el que no se
construyó: una compilación de la tienda más antigua o más reciente
sencillamente conserva su propio código.

## Suelo de versión { #release-floor }

Una firma dice quién publicó una versión, no que sea reciente. Sin nada más,
cualquier servidor, o cualquiera en el camino de una conexión HTTP sin cifrar,
podría entregar a una app una versión firmada antigua y traer de vuelta un
fallo corregido desde entonces.

- Una versión está fechada por el commit desde el que se construyó
  (`createdAt` en el manifiesto firmado, fecha del committer).
- Cada dispositivo mantiene un suelo: la más reciente entre la fecha del
  commit desde el que se hizo su compilación de la tienda (registrada por
  Gradle) y la de la versión servida más reciente que se ha mostrado en él. El
  suelo solo sube.
- La app no descarga nada por debajo del suelo y arranca la versión más
  reciente que tiene a esa altura o por encima, venga del servidor que venga.
  Un servidor que no sirve nada, o algo más antiguo, deja a la app con lo que
  tiene.
- El código integrado en la app siempre puede ejecutarse. Es el que arranca
  cuando nada de lo almacenado sirve. También está fechado por su commit, no
  por el momento en que se compiló la app, de modo que la versión del mismo
  commit, y de cualquier commit posterior, se toma en su lugar.
- El JavaScript mantiene el mismo suelo por su parte y se niega a ejecutarse
  por debajo.

Lo que se deduce de ello:

- Frente a un servidor más antiguo, una app ejecuta un JavaScript más reciente
  que el que ese servidor traía, como hace una compilación de la tienda. La
  API del servidor tiene que seguir siendo compatible con clientes más
  recientes.
- Cuando el guardián descarta una versión que ya se había mostrado, la
  anterior suele estar por debajo del suelo: la app ejecuta su código
  integrado hasta que la instancia publica una versión más reciente.
- Una compilación cuyo suelo no se puede leer no acepta ninguna actualización.
- Dos compilaciones de un mismo commit son dos versiones con casi la misma
  fecha. Las apps almacenan una versión por fecha: si las dos fechas
  coinciden, la segunda no se almacena y la app conserva lo que ejecuta.
  Construye una sola vez por versión.

Nada sin firmar dirige todo esto: las apps ignoran los filtros de manifiesto,
las cabeceras definidas por el servidor, la parte de extensiones y las
directivas, y no envían a los servidores ni un identificador de instalación ni
el texto de un cierre inesperado anterior.

## Cuando una actualización se comporta mal { #when-an-update-misbehaves }

- Un bundle que falla antes de mostrarse por primera vez lo descarta
  expo-updates, que vuelve al anterior o al código integrado en la app.
- Un bundle que se muestra y luego muere en cada inicio lo atrapa el guardián
  de la propia app (`clientUpdateBootGuard`): tras tres inicios seguidos que
  terminaron sin estabilizarse, el siguiente pide la misma vuelta atrás.
- Un bundle descartado no se vuelve a probar. La instancia tiene que publicar
  otro.
- Cuando el bundle descartado no deja nada que arrancar (el anterior está por
  debajo del suelo), la app se reinicia sola con su código integrado.

## El parche de expo-updates { #the-expo-updates-patch }

`frontend/patches/expo-updates@*.patch` lleva lo que el diseño necesita y para
lo que expo-updates no tiene ningún ajuste:

- las direcciones de los assets de un manifiesto pueden ser relativas,
  resueltas contra la dirección del manifiesto, de modo que un mismo
  manifiesto firmado es válido en todas las instancias;
- la petición de vuelta atrás del guardián descarta una actualización incluso
  cuando ya ha arrancado con éxito antes;
- cuando eso no deja nada que arrancar, la app se reinicia en lugar de fallar;
- el suelo de versión, y el arranque por versión en lugar de por dirección de
  servidor;
- el código integrado en la app está fechado por su commit;
- una versión ya almacenada se adopta, no se descarga de nuevo, cuando otra
  dirección la sirve (la misma instancia desde la red local y desde fuera);
- cada asset debe llevar su hash en el manifiesto firmado;
- los certificados se comprueban en el más tardío entre el reloj del
  dispositivo y el suelo, y una versión no puede estar fechada después de la
  caducidad de su certificado de firma;
- las entradas sin firmar y las cabeceras identificativas citadas arriba se
  eliminan.

Vuelve a crear el parche cuando se actualice expo-updates; la dependencia está
fijada a la versión exacta parcheada.

## Lo que una compilación debe tener { #what-a-build-must-have }

`frontend/scripts/check-expo-updates-patch.mjs` comprueba, para las dos apps:
que el parche está en las fuentes que compila Android, que el certificado y
sus metadatos están configurados (sin certificado, expo-updates no verifica
nada), que el certificado es una raíz y las apps verifican a través de ella,
que la compilación registra su suelo de versión, que ningún ajuste amplía lo
que las apps aceptan, y que la versión del runtime nativo coincide con
`app.json`. Se ejecuta antes de cada compilación Gradle de las apps, y en el
gate en cualquier rama que toque las dependencias, sus parches, el certificado
o el lado nativo de una app.

## Claves { #keys }

Dos claves, creadas por `frontend/scripts/client-update-certificate.sh`:

- **La raíz** (`root <dir>`), una sola vez. Su certificado es
  `frontend/client-updates/certificate.pem`, compilado en cada compilación de
  la tienda; su clave está cifrada con una frase de contraseña y permanece
  fuera de línea, con una copia de seguridad. Nunca va a una máquina de
  compilación. Perderla, o tener que sustituirla, implica una nueva
  compilación de la tienda de las dos apps antes de poder servir de nuevo una
  actualización.
- **El certificado de firma** (`leaf <dir> [days]`), un año por defecto,
  emitido por la raíz. Su clave es la que usa una compilación de publicación.

Renueva el certificado de firma antes de que caduque: la herramienta de
publicación rechaza uno al que le quedan menos de 30 días, y los dispositivos
rechazan una versión en cuanto su certificado ha caducado, de modo que una
imagen deja de entregar su bundle un año después de la emisión de su
certificado (los dispositivos conservan lo que ya tienen). No hay revocación:
una clave de firma filtrada vale hasta que caduca su certificado, y solo para
versiones fechadas antes. Sustituir la raíz es el único corte más temprano.

## Publicar una versión { #releasing }

1. `EVEY_CLIENT_UPDATE_KEY_FILE=<signing key> EVEY_CLIENT_UPDATE_CERT_FILE=<signing certificate> make client-updates`
   una vez, en el commit de la versión con el árbol limpio, antes de construir
   la imagen. El bytecode de Hermes no es reproducible: dos ejecuciones dan
   dos actualizaciones distintas, así que todas las arquitecturas de una
   versión deben llevar la misma salida. La herramienta se niega a publicar
   por encima de una versión más reciente.
2. Construye la imagen. La construcción falla con un bundle sin firmar,
   construido desde otro commit, o destinado a un runtime que las apps ya no
   declaran.

La CI lo hace solo para las etiquetas de versión (`v*.*.*`), en dos jobs
(`.github/workflows/ci.yml`):

- uno construye los bundles sin firmar, con todas las dependencias de las apps
  y sin ninguna clave;
- otro los firma, con la clave y nada más que un checkout:
  `frontend/scripts/sign-client-update.mjs` no carga ninguna dependencia, y
  rechaza un bundle que no sea el del commit etiquetado, para los runtimes que
  las apps declaran en ese commit, fechado por ese commit, con archivos que
  son lo que dice el manifiesto.

La construcción de la imagen de cada arquitectura toma después esa misma
salida firmada. Una etiqueta cuyos bundles no se pueden firmar falla, no sale
sin ellos.

Lo que la firma no cubre: el job de construcción ejecuta las dependencias de
las apps, y una de ellas podría alterar el JavaScript antes de que se firme.
El job de firma comprueba la forma, el commit y la fecha de un bundle, no lo
que hace su bytecode. La defensa es el lockfile congelado y la revisión de
cada subida de dependencia.

Las imágenes construidas desde una rama o a mano no llevan ningún bundle. Un
bundle firmado para una rama sería aceptado por todas las apps publicadas,
desde cualquier servidor, y subiría su suelo por encima de las versiones.

El job de firma se ejecuta en el entorno de GitHub `client-update-signing`,
que guarda los dos secretos `EVEY_CLIENT_UPDATE_SIGNING_KEY` y
`EVEY_CLIENT_UPDATE_SIGNING_CERT` (dos archivos PEM, enteros) y solo deja
entrar las etiquetas `v*.*.*`: una rama que modifica el workflow no puede
leerlos. Sustituye los dos cuando se renueve el certificado de firma.

Ninguna clave privada entra nunca en el repositorio ni en una imagen, y las
claves solo se pasan como rutas de archivo.

<!-- sources
routes and what the server serves: internal/adapters/driving/http/client_update_handler.go ; internal/adapters/driving/http/client_update_handler_test.go ; internal/config/config.go - EVEY_CLIENT_UPDATES_DIR
following the active server, restart prompt, launch guard: frontend/packages/client/src/native/clientUpdates.ts - followClientUpdateSource, clientUpdateBootGuard, clientUpdateMarkSettled ; frontend/packages/client/src/native/clientUpdates.test.ts ; i18n common.clientUpdate.*
floor, launch by release, certificates, dropped inputs: frontend/patches - the expo-updates patch
build checks: frontend/scripts/check-expo-updates-patch.mjs ; Dockerfile
keys and publication: frontend/scripts/client-update-certificate.sh ; frontend/scripts/build-client-update.mjs ; frontend/scripts/sign-client-update.mjs ; .github/workflows/ci.yml (client-updates-build, client-updates-sign)
-->
