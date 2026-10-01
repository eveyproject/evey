---
source_sha: c9ccdddebbef17e96ff8f3da3475c803b0d09dc237cd4c140c897bdfdaf289c5
reviewed: false
---
# Escribir un plugin

Recorrido completo para quien acaba de clonar Evey y quiere escribir, probar y publicar un plugin o un tema. Elige el entorno de ejecución que corresponde al lenguaje de tu plugin; el resto es común.

## 1. Requisitos previos

Evey ejecuta el proceso anfitrión (Go) y distribuye los plugins como ZIP. El entorno de ejecución del plugin depende de lo que llame tu código.

### Siempre necesario

| Herramienta | Por qué | Instalación |
|---|---|---|
| Go ≥ 1.27 | Compilar el anfitrión Evey | `https://go.dev/dl/` |
| Docker + Docker Compose | Entorno de desarrollo por defecto (Postgres, Meilisearch, servidor, web) | https://docs.docker.com/engine/install/ |
| `bubblewrap` (`bwrap`) | Sandbox para los procesos de plugins, necesario para los modos `bubblewrap` y `strict` | `apt install bubblewrap` (Debian/Ubuntu), `dnf install bubblewrap` (Fedora), `pacman -S bubblewrap` (Arch). macOS: instala Linux en una máquina virtual, bwrap solo existe en Linux. |

El modo de sandbox `basic` funciona sin bwrap pero no debería usarse nunca fuera de experimentos locales desechables: déjalo en cuanto puedas.

### Por lenguaje

Elige lo que tu plugin usa de verdad; no los necesitas todos.

| Lenguaje del plugin | Herramientas | Notas |
|---|---|---|
| Python 3 | `python3`, `pip` | Usa `pip install --user` para mantener limpio el equipo. Los stubs del SDK están en `sdk/python/`. |
| Node.js | Node 20+, `npm` o `pnpm` | El SDK carga `plugin_api.proto` en tiempo de ejecución mediante `@grpc/proto-loader`. |
| Go | Go ≥ 1.21 | Sin stubs incluidos: genéralos con `protoc-gen-go` + `protoc-gen-go-grpc`; un binario Go compilado necesita además el permiso `native-binary`. |
| Dart / Java / PHP / Ruby / Swift | Herramientas de cada lenguaje | Los stubs vienen generados en `sdk/<lang>/`. |
| Tema solo CSS | Nada más | Un tema es un ZIP con `kind: theme` + un `theme.css`. |
| JS de tema | Nada más (el anfitrión ejecuta tu JS dentro de un `iframe` en sandbox con `sandbox="allow-scripts"` y una CSP cerrada) | El analizador estático rechaza las cookies, el almacenamiento, las llamadas de red y el acceso a la página anfitriona (`document.cookie`, `localStorage`, `fetch(`, `window.parent`...): consulta [Temas](../themes.md#what-the-scanner-refuses). |

### Generar stubs para otro lenguaje

Los stubs vienen generados para los lenguajes de arriba, pero el contrato gRPC de
los plugins es solo un archivo proto: puedes apuntar a **cualquier lenguaje que
admita `protoc`** (Rust, C#, Kotlin, Elixir, …). La única fuente de verdad es:

```
proto/plugin/v1/plugin_api.proto
```

Genéralos con `protoc` (o `buf`), apuntando a él el plugin gRPC de tu lenguaje:

```bash
# forma genérica: las opciones --*_out exactas dependen del lenguaje o plugin
protoc --proto_path=proto \
       --<lang>_out=OUT_DIR --grpc_<lang>_out=OUT_DIR \
       proto/plugin/v1/plugin_api.proto
```

Las recetas para los lenguajes incluidos (qué plugin gRPC necesita cada uno:
`grpc_php_plugin`, `protoc-gen-grpc-java`, `protoc-gen-grpc-swift`, …) están en el
objetivo `plugin-sdk` del `Makefile` raíz y en cada `sdk/<lang>/README.md`
(por ejemplo `sdk/php/README.md`). `make plugin-sdk` regenera los stubs incluidos.

Incluye los stubs generados dentro del ZIP de tu plugin: el anfitrión solo copia
automáticamente los stubs de los lenguajes que incluye (`sdk/<lang>/`).

### Opcional pero recomendado

| Herramienta | Por qué |
|---|---|
| `cosign` (Sigstore) | Firmar el ZIP de tu plugin. `EVEY_PLUGINS_COSIGN_POLICY` vale `warn` por defecto (no `off`): con `warn` y sin clave pública configurada, la verificación no hace nada (se registra un aviso, no se bloquea nada); define `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` para verificar de verdad las firmas, y la política en `enforce` para rechazar las subidas sin firmar (`enforce` sin clave pública detiene el servidor al arrancar en lugar de aceptar todos los plugins sin verificar). https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | Detectar los problemas evidentes antes de enviar un plugin. |
| `golangci-lint` | Necesario si tu plugin está en Go y quieres el mismo linting que el anfitrión. |

## 2. Levantar Evey en local

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # genera una configuración local razonable
make dev                        # docker compose up: postgres, meilisearch, servidor (Air), web (Vite)
```

Cuando el servidor esté sano, abre https://evey.test (lo sirve Caddy). El asistente de configuración crea la cuenta de administrador en el primer arranque, como se describe en [Primera configuración](../getting-started/first-setup.md).

Si `make dev` falla por `bwrap`:
- Confirma que `which bwrap` devuelve una ruta en el equipo
- Dentro de Docker, el contenedor de desarrollo incluye el binario bwrap; si personalizaste el compose, móntalo o define `EVEY_PLUGINS_SANDBOX_MODE=basic` *solo para el contenedor de desarrollo*

## 3. Crear el esqueleto de tu primer plugin

Los ejemplos están en `examples/plugins/`: plantillas en Node.js y Python (`nodejs-example`, `python-example`), un proveedor de metadatos (`metadata-example`), un receptor de webhooks (`webhook-receiver`), un widget de inicio (`weather`), configuración de plugin (`config-demo`) y dos temas (`theme-dark-blue`, `open-workshop`). Elige el más cercano a tu caso y cópialo en un directorio de trabajo. Para la persistencia y los permisos, consulta **`examples/plugins/data-demo`**: usa el almacén de datos propio del plugin y pide los permisos `public` / `prefs` + el slot `admin.nav`.

```sh
cp -r examples/plugins/nodejs-example ~/my-first-plugin
cd ~/my-first-plugin
```

Edita `plugin.yaml`:

- `name`: kebab-case, único
- `version`: semver, súbela en cada versión
- `kind`: `plugin` (backend obligatorio) o `theme` (solo frontend)
- `language`: `python` | `nodejs` | `go` | `php` | `ruby` | `lua` | `bash` | `sh` (identificador del manifiesto, no el nombre del binario del intérprete; `python3`/`node` no son valores válidos de `language`, ver `entrypoint` abajo).
- `entrypoint`: la línea de comandos que ejecuta el entorno
- `permissions`: declara cada API del anfitrión que llamas. El instalador rechaza las llamadas no declaradas. Consulta el [catálogo de permisos](security-model.md#permissions-catalog) para la lista completa.
- `external_fetch`: cuando necesitas hablar con una API de terceros. Las entradas son nombres de host simples (`api.example.com`): ni se exige ni se usa esquema (cualquier prefijo `https://` se analiza y solo se conserva el nombre de host), sin comodines (`*.example.com` se rechaza al instalar: `external_fetch entry %q is a wildcard - use explicit hostnames`), sin direcciones IP literales (ninguna, no solo las RFC1918). El validador está en `internal/adapters/driven/pluginrunner/manifest.go` (`validateExternalFetch`); sus pruebas están en `internal/adapters/driven/pluginrunner/manifest_test.go`: ejecuta `go test ./internal/adapters/driven/pluginrunner/...` para comprobar que tu valor es válido.

Módulo frontend (opcional para `kind: plugin`, prohibido para `kind: theme`):

```yaml title="plugin.yaml"
frontend:
  - slot: home.section
    src: dist/index.js
```

El slot debe existir en `internal/domain/plugin/slots.go` (`home.section`, `home.hero`, `library.sidebar`, `media.detail.sidebar`, `player.overlay`, `sidebar.bottom`, `settings.user.appearance`, y `admin.nav`, el único slot controlado de la zona de administración, una entrada de menú que abre la página propia y en sandbox de tu plugin). Cualquier otro (inyección en `admin.*`, `login.*`, `setup.*`, `profile.security.*`) se rechaza al instalar.

**Guardar datos.** Tu backend tiene un almacén clave/valor privado a través del
socket del anfitrión (`SetData`/`GetData`/`DeleteData`/`ListDataKeys`): no hace
falta ningún permiso, y se borra automáticamente al desinstalar. Para las
preferencias por usuario desde tu iframe, pide el permiso `prefs`; para leer
medios no sensibles (título, póster), pide `public`. Consulta la
[referencia de la API](api-reference.md#permissions). Los ámbitos de
administración, autenticación, sistema y escritura genérica no se pueden pedir
nunca.

## 4. El ciclo de instalación

```sh
# comprime tu plugin
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# súbelo desde la interfaz de administración
open https://evey.test/admin/plugins
# o mediante la API
curl -F "plugin=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins/upload
# una firma va en un segundo campo: -F "sig=@my-first-plugin.zip.sig"
# una actualización que pide permisos nuevos: -F "accept_permission_changes=true"
```

El instalador sigue este orden, y cada paso puede rechazar la instalación:

1. **Validación del manifiesto**: `plugin.yaml` se lee directamente del ZIP (antes de las comprobaciones del ZIP de abajo) y se valida su esquema. Lista de slots permitidos. Lista de permisos y capacidades permitidos. Analizador de `external_fetch` (rechaza los comodines y cualquier dirección IP literal; un prefijo de esquema como `http://` se acepta pero se ignora: solo se conserva el nombre de host). El entorno de ejecución del `language` declarado debe estar disponible en la imagen.
2. **Escalada de permisos** (solo actualización): el instalador compara el nuevo manifiesto con la versión anterior. Cualquier permiso, capacidad, slot o host de external_fetch nuevo bloquea la instalación con un error que quien llama debe anular de forma explícita (`accept_permission_changes: true` desde el frontend). Las instalaciones nuevas se saltan este paso: no hay nada con qué comparar.
3. **Firma cosign**: se verifica cuando hay un verificador de firmas configurado; un fallo solo bloquea la instalación cuando `EVEY_PLUGINS_COSIGN_POLICY=enforce`. Consulta la sección 6.
4. **Extracción del ZIP**: extensión, tamaño total ≤ 500 MB, tamaño por entrada ≤ 100 MB, ratio de compresión ≤ 100:1, sin enlaces simbólicos, sin entradas que salgan del archivo; los archivos van a un directorio de preparación (una actualización nunca toca la versión activa hasta que pasan todos los pasos siguientes).
5. **Saneador de CSS** (solo temas): elimina `@import`, bloquea `url()` que no sea `data:`, bloquea las superposiciones a pantalla completa, bloquea los selectores que apuntan a `[type=password]` y `.admin-*`, limita `z-index` a ≤ 999.
6. **Analizador del JS de temas** (solo temas): rechaza cualquier referencia a `document.cookie`, `localStorage`, `fetch`, `eval`, `globalThis`, el acceso por corchetes a propiedades de los objetos globales del anfitrión, `constructor.constructor`, etc. Los temas solo deben usar la API `evey.theme.*`.
7. **Análisis estático**: revisa cada archivo de texto en busca de ejecución de shell, shell inversa, eval, decodificación base64 seguida de ejecución, patrones de ofuscación, comandos de instalación en tiempo de ejecución y escalada de privilegios. Hallazgos críticos = instalación rechazada. Los hallazgos de aviso (deserialización, ofuscación, native_code) pasan a rechazo salvo que se declare el permiso correspondiente.
8. **Persistencia**: pone el directorio de preparación en su sitio, escribe la fila en Postgres y, si procede, lanza el entorno de ejecución.

Si algún paso falla, la interfaz de administración muestra el motivo exacto. Los registros del servidor (`docker compose logs evey`) llevan la traza completa.

## 5. Iterar

Recarga tu plugin en caliente mientras desarrollas:

```sh
# editar el código → volver a comprimir → volver a subir desde la interfaz (botón Actualizar)
# o, más rápido: deja el archivo directamente en el directorio del plugin dentro del contenedor
docker compose exec evey ls /data/plugins/<plugin-id>/
```

Una subida solo se acepta como actualización cuando su `version` es mayor que la instalada: súbela en cada iteración (una versión igual o menor se rechaza), o desinstala antes. Si añades un permiso nuevo, entrarás en el flujo de escalada: es el diseño, no un fallo.

Los registros del proceso del plugin acaban en:
- `/admin/plugins/<id>`: las últimas N líneas en la interfaz
- `docker compose logs evey | grep <plugin-id>`: el flujo completo

## 6. Firmar tu plugin (opcional en desarrollo, recomendado para distribuirlo)

Si piensas distribuir el plugin a otras instancias de Evey, fírmalo. El administrador que lo recibe puede usar entonces `EVEY_PLUGINS_COSIGN_POLICY=warn` (o `enforce`) para detectar las subidas sin firmar.

```sh
# una sola vez: genera un par de claves (guarda bien cosign.key, nunca lo subas al repositorio)
cosign generate-key-pair

# firma el ZIP
cosign sign-blob \
  --key cosign.key \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip

# distribuye: my-first-plugin.zip + my-first-plugin.zip.sig + cosign.pub
```

En la instancia de Evey que instala el plugin:

```sh
# pon la clave pública en un sitio que el servidor pueda leer
sudo cp cosign.pub /etc/evey/cosign.pub

# apunta Evey a ella y cambia la política
export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # o "enforce" para rechazar lo no firmado
docker compose restart evey
```

Evey valida la clave al arrancar (análisis PEM): un archivo de clave ausente o mal formado hace fallar el arranque en lugar de la primera instalación. Las referencias KMS (`gcpkms://...`, `awskms://...`, `k8s://...`) se aceptan tal cual.

## 7. Límites de recursos y sandbox

Valores por defecto aplicados a cada proceso de plugin:

| Variable de entorno | Por defecto | Efecto |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` = elige el mejor disponible (normalmente `landlock`). `landlock` = lista de permitidos del LSM Landlock + bloqueo incondicional de la salida TCP (`-restrict-net` + seccomp `-deny-inet`, sin excepción para el permiso `network`) + seccomp LD_PRELOAD. `basic` = seccomp LD_PRELOAD + bajada de UID únicamente (depuración). `bubblewrap` = espacios de nombres completos + raíz montada en solo lectura + `--unshare-net` (también incondicional, para todos los plugins, tengan o no el permiso `network`). `strict` = bubblewrap + el shim seccomp LD_PRELOAD. |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | RSS máximo. El monitor de recursos consulta `/proc` y detiene el plugin si lo supera. `0` lo desactiva. |
| `EVEY_PLUGINS_CPU_NICE` | 10 | Valor `nice` (0-19) aplicado al lanzar, para que un plugin nunca pueda dejar sin CPU al servidor. |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | Número máximo de hilos y procesos. Evita las bombas fork. `0` lo desactiva. |
| `EVEY_PLUGINS_DISK_MAX_MB` | 100 | Tamaño máximo del directorio de instalación; el monitor lo mide cada ~30 s y detiene un plugin que lo supera. `0` lo desactiva. |

Un plugin que habla con la red necesita a la vez `permissions: [network]` y `external_fetch: [<host>...]`. La salida directa a la red se corta en el sandbox para todos los plugins, de forma incondicional, sean cuales sean sus permisos (ver los modos de sandbox arriba): el propio proceso del plugin nunca puede abrir un socket hacia internet. La única salida es el intermediario RPC `Fetch` del lado del anfitrión: el plugin lo llama por el socket Unix `EVEY_PLUGIN_SOCK`, el anfitrión comprueba el host de destino con `external_fetch` y después conecta mediante un cliente protegido contra SSRF (IP privadas y reservadas rechazadas al conectar, redirecciones validadas de nuevo con la IP resuelta y con el host permitido). Consulta `internal/adapters/driving/pluginhost/fetch.go`.

## 8. Distribuir un plugin

Todavía no hay catálogo oficial: un administrador instala un plugin subiendo su
ZIP en el panel de administración, desde donde lo publiques (una release de
GitHub, tu propio sitio). Lo que significa hoy "distribuir":

- Entrega juntos el ZIP, su `.sig` y tu `cosign.pub` (sección 6), para que el
  administrador que lo recibe pueda verificar la subida con
  `EVEY_PLUGINS_COSIGN_POLICY=warn` o `enforce`.
- Indica los permisos y los hosts de `external_fetch` en tu README: el
  administrador los ve al instalar y tiene que aprobarlos; una sorpresa ahí es el
  primer motivo de rechazo de una instalación.
- Para que tu plugin aparezca junto a los ejemplos incluidos, abre una PR contra
  `examples/plugins/` con el código fuente y un README. Los ejemplos se revisan y
  se compilan desde las fuentes; una PR no es un respaldo y no lleva ninguna firma
  del proyecto.

## 9. Solución de problemas

| Síntoma | Causa | Solución |
|---|---|---|
| `bwrap: ... permission denied` | Modo `bubblewrap` dentro de Docker: los perfiles seccomp y AppArmor por defecto bloquean las llamadas `unshare` y `mount` que necesita. | Mantén el modo por defecto `auto`, que usa Landlock en un contenedor. Si no, aplica el perfil incluido `docker/seccomp-evey.json` (también en la imagen en `/etc/evey/seccomp.json`), una rebaja de seguridad deliberada: consulta [Configurar el sandbox de plugins](sandbox-deployment.md). |
| `forbidden pattern "fetch(" found in theme JS` | El analizador del JS de temas detectó una API prohibida. | Los temas solo deben llamar a `evey.theme.*`. Lee `internal/pkg/themejs/scan.go` para ver lo que se bloquea. |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard - use explicit hostnames` | Todos los comodines se rechazan al instalar, sin excepción. | Usa `api.tmdb.org` (y lista cada subdominio explícito que necesites), no `*.tmdb.org` ni `cdn.*.tmdb.org`. |
| `manifest: frontend slot "admin.section" not allowed` | Los frontends de plugins tienen prohibidas las zonas privilegiadas. | Elige un slot no crítico de la lista de permitidos o replantea el plugin. |
| `cosign verify-blob: failed signature` | La clave pública del servidor no corresponde a la clave privada usada para firmar. | Vuelve a firmar con la clave correcta, o actualiza `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`. |
| `plugin already installed at version 1.0.0, new version must be higher` | El ZIP lleva la versión ya instalada. | Sube `version` en `plugin.yaml`, o desinstala antes el plugin. |
| El plugin se para y el registro dice `memory quota exceeded (… MB > … MB)` | El monitor de recursos lo mató al superar su límite de memoria (salida 137). | Reduce el uso de memoria, o sube el límite de memoria del plugin en su tarjeta de **Admin → Plugins** (`EVEY_PLUGINS_MEMORY_MB` fija el valor por defecto). |
| El módulo frontend no se ve | Plugin no activado, slot que no corresponde, o prefijo `web_root` erróneo en `frontend.src`. | Revisa la página Plugins de administración (insignia de estado), confirma que la cadena del slot corresponde a un montaje `<PluginSlot name=...>` real, mira en el panel de red las llamadas `/api/v1/plugins/{id}/assets/...`. |

## 10. Para ir más lejos

- El modelo de seguridad de principio a fin: [security-model.md](security-model.md)
- La superficie de la API del anfitrión: [api-reference.md](api-reference.md)
- Los slots disponibles: [api-reference.md](api-reference.md#slots)
- El código del SDK por lenguaje: `sdk/`
- Plugins y temas de referencia: `examples/plugins/`
