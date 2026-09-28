---
source_sha: d5cb641722482071362563f54ebfa269c77cb3d81faa2c0ec9878ad7f4ef501c
---
# Preparación para desarrollar extensiones

Recorrido completo para quien acaba de clonar Evey y quiere escribir, probar y publicar una extensión o un tema. Elija el entorno de ejecución que corresponda al lenguaje de su extensión; el resto es común.

## 1. Requisitos previos

Evey ejecuta el proceso anfitrión (Go) y distribuye las extensiones como ZIP. El entorno de ejecución de la extensión depende de lo que llame su código.

### Siempre necesarios

| Herramienta | Para qué | Instalación |
|---|---|---|
| Go ≥ 1.25 | Compilar el anfitrión Evey | `https://go.dev/dl/` |
| Docker + Docker Compose | Pila de desarrollo por defecto (Postgres, Meilisearch, servidor, web) | https://docs.docker.com/engine/install/ |
| `bubblewrap` (`bwrap`) | Entorno aislado para los procesos de las extensiones - necesario para los modos `bubblewrap` y `strict` | `apt install bubblewrap` (Debian/Ubuntu), `dnf install bubblewrap` (Fedora), `pacman -S bubblewrap` (Arch). macOS: instale Linux en una VM, bwrap solo existe en Linux. |

El modo de aislamiento `basic` funciona sin bwrap, pero nunca debería usarse fuera de experimentos locales desechables - abandónelo en cuanto pueda.

### Según el lenguaje

Tome lo que su extensión use de verdad; no le hace falta todo.

| Lenguaje de la extensión | Herramientas | Notas |
|---|---|---|
| Python 3 | `python3`, `pip` | Use `pip install --user` para mantener limpio el anfitrión. Los stubs del SDK están en `sdk/python/`. |
| Node.js | Node 20+, `npm` o `pnpm` | El SDK carga `plugin_api.proto` en tiempo de ejecución mediante `@grpc/proto-loader`. |
| Go | Go ≥ 1.21 | Sin stubs incluidos - genérelos con `protoc-gen-go` + `protoc-gen-go-grpc`; un binario Go compilado necesita además el permiso `native-binary`. |
| Dart / Java / PHP / Ruby / Swift | La cadena de herramientas de cada lenguaje | Los stubs vienen ya generados en `sdk/<lang>/`. |
| Tema solo CSS | Nada más | Un tema es un ZIP con `kind: theme` + un `theme.css`. |
| Tema con JS | Nada más (el anfitrión ejecuta su JS dentro de un `iframe` aislado con `sandbox="allow-scripts"` y una CSP cerrada) | El escáner estático rechaza todo acceso a `document`, `window`, `fetch`, `localStorage`, etc. - vea `internal/pkg/themejs/scan.go`. |

### Generar stubs para otro lenguaje

Los stubs vienen ya generados para los lenguajes de arriba, pero el contrato gRPC
de las extensiones es solo un archivo proto - puede usar **cualquier lenguaje que
admita `protoc`** (Rust, C#, Kotlin, Elixir, …). La única fuente de verdad es:

```
proto/plugin/v1/plugin_api.proto
```

Genere con `protoc` (o `buf`), apuntando a él el plugin gRPC de su lenguaje:

```bash
# generic shape - exact --*_out flags depend on the language/plugin
protoc --proto_path=proto \
       --<lang>_out=OUT_DIR --grpc_<lang>_out=OUT_DIR \
       proto/plugin/v1/plugin_api.proto
```

Las recetas probadas para los lenguajes incluidos (qué plugin gRPC necesita cada
uno: `grpc_php_plugin`, `protoc-gen-grpc-java`, `protoc-gen-grpc-swift`, …) están
en el objetivo `plugin-sdk` del `Makefile` raíz y en cada `sdk/<lang>/README.md`
(p. ej. `sdk/php/README.md`). `make plugin-sdk` regenera los stubs distribuidos.

Incluya los stubs generados dentro del ZIP de su extensión - el anfitrión solo
copia automáticamente los stubs de los lenguajes que trae (`sdk/<lang>/`).

### Opcional pero recomendado

| Herramienta | Para qué |
|---|---|
| `cosign` (Sigstore) | Firmar el ZIP de su extensión. `EVEY_PLUGINS_COSIGN_POLICY` vale `warn` por defecto (no `off`) - con `warn` y sin clave pública configurada, la verificación no hace nada (se registra un aviso, no se bloquea nada); defina `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` para verificar de verdad las firmas, o `enforce` para rechazar las subidas sin firmar. https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | Detectar problemas evidentes antes de enviar una extensión. |
| `golangci-lint` | Necesario si su extensión está en Go y quiere el mismo nivel de lint que el anfitrión. |

## 2. Arrancar Evey en local

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # generates a sane local config
make dev                        # docker compose up: postgres, meilisearch, server (Air), web (Vite)
```

Cuando el servidor esté sano, abra https://evey.test (lo sirve Caddy). Las credenciales por defecto están en `.env.dev` - cámbielas antes de exponer la instancia.

Si `make dev` falla por `bwrap`:
- Confirme que `which bwrap` devuelve una ruta en el anfitrión
- Dentro de Docker, el contenedor de desarrollo trae el binario bwrap; si ha personalizado la pila Compose, móntelo o ponga `EVEY_PLUGINS_SANDBOX_MODE=basic` *solo para el contenedor de desarrollo*

## 3. El esqueleto de su primera extensión

Los ejemplos están en `examples/plugins/`: puntos de partida en Node.js y Python (`nodejs-example`, `python-example`), una fuente de metadatos (`metadata-example`), un receptor de webhooks (`webhook-receiver`), un widget de inicio (`weather`), la configuración de una extensión (`config-demo`) y dos temas (`theme-dark-blue`, `open-workshop`). Elija el más cercano a su caso y cópielo en un directorio de trabajo. Para el almacenamiento y los permisos, vea **`examples/plugins/data-demo`** - usa el almacén de datos por extensión y pide los permisos `public` / `prefs` además de la ranura `admin.nav`.

```sh
cp -r examples/plugins/nodejs-example ~/my-first-plugin
cd ~/my-first-plugin
```

Edite `plugin.yaml`:

- `name`: kebab-case, único
- `version`: semver - increméntela en cada publicación
- `kind`: `plugin` (necesita backend) o `theme` (solo interfaz)
- `language`: `python` | `nodejs` | `go` | `php` | `ruby` | `lua` | `bash` | `sh` (identificador del manifiesto - no el nombre del binario del intérprete; `python3`/`node` no son valores válidos de `language`, vea `entrypoint` abajo).
- `entrypoint`: la línea de órdenes que ejecuta el entorno
- `permissions`: declare cada API del anfitrión que llame. El instalador rechaza las llamadas no declaradas. La lista completa está en el [catálogo de permisos](security-model.md#permissions-catalog).
- `external_fetch`: cuando necesite hablar con una API de terceros. Las entradas son nombres de host sin más (`api.example.com`) - no se requiere ni se usa esquema (un prefijo `https://` se analiza y solo se conserva el host), sin comodines (`*.example.com` se rechaza en la instalación: `external_fetch entry %q is a wildcard - use explicit hostnames`), sin direcciones IP literales (ninguna, no solo las RFC1918). El validador está en `internal/adapters/driven/pluginrunner/manifest.go` (`validateExternalFetch`); sus pruebas en `internal/adapters/driven/pluginrunner/manifest_test.go` - ejecute `go test ./internal/adapters/driven/pluginrunner/...` para comprobar que su valor encaja.

Módulo frontal (opcional para `kind: plugin`, prohibido para `kind: theme`):

```yaml title="plugin.yaml"
frontend:
  - slot: home.section
    src: dist/index.js
```

La ranura debe existir en `internal/domain/plugin/slots.go` (`home.section`, `home.hero`, `library.sidebar`, `media.detail.sidebar`, `player.overlay`, `sidebar.bottom`, `settings.user.appearance`, y `admin.nav` - la única ranura controlada del área de administración, una entrada de menú que abre la página aislada propia de su extensión). Cualquier otra (inyección `admin.*`, `login.*`, `setup.*`, `profile.security.*`) se rechaza en la instalación.

**Guardar datos.** Su backend recibe por el socket del anfitrión un almacén
clave-valor privado (`SetData`/`GetData`/`DeleteData`/`ListDataKeys`) - sin
permiso, borrado automáticamente al desinstalar. Para preferencias por usuario
desde su iframe, pida el permiso `prefs`; para leer medios no sensibles
(título/cartel), pida `public`. Vea la [referencia de la API](api-reference.md#permissions).
Los ámbitos de administración, autenticación, sistema y escritura genérica no se
pueden pedir nunca.

## 4. El ciclo de instalación

```sh
# zip your plugin
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# upload through the admin UI
open https://evey.test/admin/plugins
# or via the API
curl -F "file=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins
```

El instalador actúa en este orden - cada paso puede rechazar la instalación:

1. **Validación del manifiesto** - `plugin.yaml` se lee directamente del ZIP (antes de las comprobaciones a nivel de ZIP de abajo) y se valida contra el esquema. Lista blanca de ranuras. Lista blanca de permisos/capacidades. Analizador de `external_fetch` (rechaza comodines y cualquier IP literal; un prefijo de esquema como `http://` se acepta pero se ignora - solo se conserva el host). El entorno `language` declarado tiene que estar disponible en la imagen.
2. **Escalada de permisos** (solo en actualizaciones) - el instalador compara el nuevo manifiesto con la versión anterior. Cualquier permiso/capacidad, ranura o host `external_fetch` nuevo bloquea la instalación con un error que quien llama debe anular explícitamente (`accept_permission_changes: true` desde la interfaz). Las instalaciones nuevas se saltan este paso - no hay nada con qué comparar.
3. **Firma cosign** - se verifica cuando hay un verificador de firmas configurado; un fallo solo bloquea la instalación con `EVEY_PLUGINS_COSIGN_POLICY=enforce`. Vea la sección 6.
4. **Extracción del ZIP** - extensión, tamaño total ≤ 500 MB, tamaño por entrada ≤ 100 MB, relación de compresión ≤ 100:1, sin enlaces simbólicos, sin entradas que salgan de la ruta; los archivos van a un directorio temporal (las actualizaciones no tocan la versión en marcha hasta que todos los pasos siguientes pasan).
5. **Saneador de CSS** (solo temas) - quita `@import`, bloquea `url()` que no sea `data:`, bloquea las superposiciones a pantalla completa, bloquea los selectores dirigidos a `[type=password]` y `.admin-*`, limita `z-index` a ≤ 999.
6. **Escáner de JS de temas** (solo temas) - rechaza toda referencia a `document.cookie`, `localStorage`, `fetch`, `eval`, `globalThis`, el acceso a propiedades con corchetes sobre globales del anfitrión, `constructor.constructor`, etc. Los temas solo pueden usar la API `evey.theme.*`.
7. **Analizador estático** - examina cada archivo de texto en busca de ejecución de shell, shell inversa, eval, decodificar base64 y ejecutar, patrones de ofuscación, órdenes de instalación en tiempo de ejecución, escalada de privilegios. Hallazgos críticos = instalación rechazada. Los avisos (deserialización, ofuscación, código nativo) pasan a rechazo salvo que se declare el permiso correspondiente.
8. **Persistencia** - pone en su sitio el directorio temporal, escribe la fila en Postgres y, si procede, lanza el entorno.

Si falla cualquier paso, la interfaz de administración muestra el motivo exacto. Los logs del servidor (`docker compose logs evey`) llevan la traza completa.

## 5. Iterar

Recargue su extensión en caliente mientras desarrolla:

```sh
# edit code → re-zip → re-upload via UI (Update button)
# or, faster: drop the file directly into the plugin directory inside the container
docker compose exec evey ls /var/lib/evey/plugins/<plugin-id>/
```

Cuando incrementa `version`, el instalador acepta la subida como actualización. Si añade un permiso nuevo, entrará en el flujo de escalada - es intencionado, no un fallo.

Los logs del proceso de la extensión van a:
- `/admin/plugins/<id>` - las últimas N líneas en la interfaz
- `docker compose logs evey | grep <plugin-id>` - el flujo completo

## 6. Firmar su extensión (opcional en desarrollo, recomendado para distribuir)

Si piensa distribuir la extensión a otras instancias de Evey, fírmela. El administrador que la reciba podrá usar `EVEY_PLUGINS_COSIGN_POLICY=warn` (o `enforce`) para hacer visibles las subidas sin firmar.

```sh
# one-time: generate a keypair (keep cosign.key safe - never commit it)
cosign generate-key-pair

# sign the ZIP
cosign sign-blob \
  --key cosign.key \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip

# distribute: my-first-plugin.zip + my-first-plugin.zip.sig + cosign.pub
```

En la instancia de Evey que instala la extensión:

```sh
# put the public key somewhere readable by the server
sudo cp cosign.pub /etc/evey/cosign.pub

# point Evey at it and switch policy
export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # or "enforce" to refuse unsigned
docker compose restart evey
```

Evey valida la clave al arrancar (análisis PEM) - un archivo de clave ausente o mal formado hace fallar el arranque en lugar de la primera instalación. Las referencias KMS (`gcpkms://...`, `awskms://...`, `k8s://...`) se aceptan tal cual.

## 7. Límites de recursos y aislamiento

Valores por defecto aplicados a cada proceso de extensión:

| Variable de entorno | Por defecto | Efecto |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` = elige lo mejor disponible (normalmente `landlock`). `landlock` = lista blanca Landlock LSM + denegación incondicional de TCP saliente (`-restrict-net` + seccomp `-deny-inet`, sin excepción para el permiso `network`) + seccomp por LD_PRELOAD. `basic` = solo seccomp por LD_PRELOAD + cambio de UID (depuración). `bubblewrap` = espacios de nombres completos + raíz montada en solo lectura + `--unshare-net` (también incondicional, para toda extensión, sea cual sea el permiso `network`). `strict` = bubblewrap + lista blanca seccomp. |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | RSS máximo. El monitor de recursos consulta `/proc` y detiene la extensión si lo supera. `0` lo desactiva. |
| `EVEY_PLUGINS_CPU_NICE` | 10 | Valor `nice` (0-19) aplicado al lanzar, para que una extensión nunca prive de CPU al servidor. |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | Número máximo de hilos/procesos. Evita las bombas fork. `0` lo desactiva. |
| `EVEY_PLUGINS_DISK_MAX_MB` | 100 | Tamaño máximo del directorio de instalación; el monitor lo mide cada ~30 s y detiene la extensión si lo supera. `0` lo desactiva. |

Una extensión que habla con la red necesita a la vez `permissions: [network]` y `external_fetch: [<host>...]`. La salida directa a la red se corta en el aislamiento para toda extensión, sin condiciones, sean cuales sean sus permisos (vea los modos de arriba) - el propio proceso de la extensión nunca puede abrir un socket hacia Internet. La única salida es el intermediario RPC `Fetch` del lado del anfitrión: la extensión lo llama por el socket Unix `EVEY_PLUGIN_SOCK`, el anfitrión comprueba el host de destino contra `external_fetch` y luego conecta con un cliente seguro frente a SSRF (IP privadas/reservadas rechazadas al conectar, redirecciones revalidadas contra la IP resuelta y el host permitido). Vea `internal/adapters/driving/pluginhost/fetch.go`.

## 8. Distribuir una extensión

Todavía no hay catálogo oficial: un administrador instala una extensión subiendo
su ZIP en el panel de administración, desde donde usted la publique (una release
de GitHub, su propio sitio). Lo que hoy significa «distribuir»:

- Entregue juntos el ZIP, su `.sig` y su `cosign.pub` (sección 6), para que el
  administrador que la reciba pueda verificar la subida con
  `EVEY_PLUGINS_COSIGN_POLICY=warn` o `enforce`.
- Indique en su README los permisos y los hosts `external_fetch` - el
  administrador los ve al instalar y tiene que aprobarlos; una sorpresa ahí es
  el primer motivo por el que se rechaza una instalación.
- Para que su extensión aparezca con los ejemplos incluidos, abra un PR contra
  `examples/plugins/` con el código fuente y un README. Los ejemplos se revisan y
  se compilan desde el código; un PR no es un respaldo y no lleva firma del
  proyecto.

## 9. Resolución de problemas

| Síntoma | Causa | Solución |
|---|---|---|
| `bwrap: ... permission denied` | El seccomp por defecto de Docker bloquea `unshare`. | El repositorio trae un perfil seccomp propio (`docker/seccomp.json`). Asegúrese de que su archivo compose lo aplica. |
| `forbidden pattern "fetch(" found in theme JS` | El escáner de JS de temas encontró una API prohibida. | Los temas solo pueden llamar a `evey.theme.*`. Lo bloqueado está en `internal/pkg/themejs/scan.go`. |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard - use explicit hostnames` | Todos los comodines se rechazan en la instalación, sin excepciones. | Use `api.tmdb.org` (y liste cada subdominio que necesite), no `*.tmdb.org` ni `cdn.*.tmdb.org`. |
| `installer: slot "admin.section" not allowed` | Los módulos frontales de las extensiones están prohibidos en las zonas privilegiadas. | Elija una ranura no crítica de la lista blanca o replantee la extensión. |
| `cosign verify-blob: failed signature` | La clave pública del servidor no corresponde a la clave privada usada para firmar. | Vuelva a firmar con la clave correcta, o actualice `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`. |
| La extensión arranca y muere con salida 137 | La cgroup la mató por falta de memoria. | Reduzca el consumo de memoria o aumente `EVEY_PLUGINS_MEMORY_MB`. |
| El módulo frontal no se ve | Extensión no activada, ranura que no coincide o prefijo `web_root` erróneo en `frontend.src`. | Revise la página Plugins de la administración (insignia de estado), confirme que el nombre de ranura corresponde a un `<PluginSlot name=...>` real, y mire en la pestaña de red las llamadas a `/api/v1/plugins/{id}/assets/...`. |

## 10. Para ir más lejos

- El modelo de seguridad de principio a fin: [security-model.md](security-model.md)
- La superficie de la API del anfitrión: [api-reference.md](api-reference.md)
- Las ranuras disponibles: [api-reference.md](api-reference.md#slots)
- Código del SDK por lenguaje: `sdk/`
- Extensiones y temas de referencia: `examples/plugins/`
