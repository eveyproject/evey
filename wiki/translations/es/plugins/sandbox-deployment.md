---
source_sha: 88e0bffcb0012a56a1138eec253a0e443e751536aff3d4d36eb018d38b7a86e9
reviewed: true
---
# Configurar el sandbox de plugins

Qué configuración de sandbox le va a tu servidor, en palabras sencillas. Para
casi todo el mundo la respuesta es: deja los valores por defecto. El servidor
detecta la máquina y elige el aislamiento más fuerte que puede. Sigue leyendo
solo si tienes un NAS antiguo, si usas Evey sin Docker o si quieres reforzar
aún más la protección. El modelo técnico completo está en
[Sandbox de plugins y modelo de amenazas](../SECURITY.md).

## Qué te protege

Un plugin que instalas queda encerrado en una caja. Según la máquina, esa
caja:

- **limita los archivos que puede tocar**: su propia carpeta y las
  bibliotecas del sistema que necesita, nada más (ni tus medios, ni tus
  secretos, ni los demás plugins);
- **bloquea las acciones peligrosas del sistema** en el kernel: depurar otros
  procesos, cargar módulos, cambiar la hora, montar discos;
- **corta el acceso directo a la red**, siempre: un plugin que pidió la red,
  y al que tú se la diste, solo llega a los servidores que declaró, y siempre
  pasando por el servidor de Evey;
- **limita la CPU, la memoria, el disco y el número de procesos**, para que
  un plugin descontrolado no pueda tumbar el servidor;
- **mantiene tus secretos ilegibles**: la clave que firma los inicios de
  sesión y las copias de seguridad.

Además, solo un administrador puede instalar plugins, se analizan en
busca de código sospechoso y tú apruebas sus permisos antes de que se
ejecuten.

**Comprueba lo que está activo:** **Administración -> Plugins -> Panel de
seguridad** muestra el sandbox del equipo, el modo que eligió `auto`
(`landlock`, `bubblewrap` o `basic`) y los resultados de las pruebas
(**Kernel Landlock**, **Lanzador sandbox**, **Bubblewrap**). `basic` significa
que el equipo no ofrece aislamiento de archivos por carpeta: los plugins se
ejecutan solo con un cambio de usuario (uid) y seccomp, las pruebas dicen por
qué, y las secciones siguientes explican qué cambiar.

## Elige tu situación

| Tu servidor | Lo que tienes sin hacer nada | ¿Algo que hacer? |
|---|---|---|
| **Linux moderno con Docker** (kernel 5.13 o posterior, es decir, casi todo lo que salió desde 2021), el caso normal | el aislamiento más fuerte (Landlock), automáticamente | nada |
| **NAS antiguo con Docker** (kernel anterior a 5.13) | buen aislamiento, sin los muros de archivos por carpeta; los secretos, la red y las acciones del sistema siguen bloqueados | nada: la separación de uid (más abajo) se activa sola |
| **Cualquier Linux, sin Docker** | el aislamiento más fuerte disponible: Landlock en un kernel moderno, si no, los espacios de nombres de bubblewrap | sigue [Sin contenedor](../deployment-bare-metal.md) |

## El único ajuste que quizá toques: la separación de uid

La separación de uid hace que un plugin se ejecute con una cuenta de usuario
distinta de la del servidor, así que físicamente no puede leer los archivos
secretos del servidor aunque nada más se lo impidiera. Solo importa en una
máquina sin los muros de archivos por carpeta (el caso `basic`); en una
máquina moderna, esos muros ya mantienen los secretos fuera de su alcance, y
la separación es una capa extra redundante.

`EVEY_PLUGINS_UID_SEPARATION`:

| Valor | Significado | Úsalo cuando |
|---|---|---|
| `auto` (por defecto) | activa solo cuando hace falta (el caso `basic`), desactivada donde es redundante | siempre, salvo que se dé uno de los dos casos siguientes |
| `on` | siempre activa, un segundo muro independiente encima de los muros de archivos | quieres protección doble y aceptas el pequeño coste descrito más abajo |
| `off` | nunca | un plugin concreto funciona mal con ella (poco frecuente) |

!!! warning
    La separación de uid es incompatible con la opción de Docker
    `no-new-privileges`: elige una de las dos. Cuando no se puede cambiar de
    usuario en una máquina antigua (`basic`), el servidor se niega a arrancar
    el plugin en lugar de ejecutarlo junto a tus secretos; en una máquina
    moderna, registra un aviso y ejecuta el plugin sin el muro extra. La
    imagen trae todo lo necesario, solo tienes que definir la variable.

El pequeño coste de `on`: el plugin se ejecuta con una cuenta aparte, así que
un plugin que escribe archivos temporales en su propia carpeta puede fallar.
Si pasa, pon `off`. Por eso el valor por defecto es `auto`: solo asume ese
riesgo donde hay un beneficio real.

## Lo que podría hacer un plugin malicioso

**Linux moderno (con Docker o sin él):** un plugin malo queda completamente
encerrado. No puede leer tus secretos, no puede ver ni tocar los archivos de
otros plugins, no puede hacer llamadas de red arbitrarias y no puede alterar
el sistema. La única salida realista sería un fallo grave del kernel, algo
que vale para cualquier sandbox que no sea una máquina virtual completa.

**NAS antiguo con Docker, sin muros por carpeta (`basic`):**

- tus secretos siguen protegidos (clave de inicio de sesión, copias de
  seguridad) gracias a la cuenta de usuario aparte, que `auto` activa aquí;
- el plugin no tiene acceso directo a la red; uno que tenga permiso solo llega
  a los servidores que declaró;
- las acciones peligrosas del sistema siguen bloqueadas en el kernel;
- pero sin muros por carpeta, un plugin malo podría leer archivos corrientes
  que no son secretos, ver la lista de procesos en marcha y, como los plugins
  comparten una misma cuenta, leer o alterar los archivos de otro plugin.

En un equipo así, instala solo plugins de confianza, o pasa a un kernel
moderno para tener todos los muros de archivos.

## Ejemplos de configuración

Esto va en tu `.env` (compose) o como variables de entorno `EVEY_*`.

```bash title="1. Servidor moderno, recomendado (nada que hacer)"
EVEY_PLUGINS_SANDBOX_MODE=auto          # elige el más fuerte disponible (por defecto)
# EVEY_PLUGINS_UID_SEPARATION=auto      # (por defecto) activa solo donde hace falta
```

```bash title="2. NAS antiguo (nada que definir, se muestra para que quede claro)"
# no pongas no-new-privileges en el security_opt del compose, bloquearía el cambio de usuario
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=auto        # (por defecto) activa en el caso basic
```

```bash title="3. Protección doble en un servidor moderno"
EVEY_PLUGINS_SANDBOX_MODE=auto          # queda en landlock
EVEY_PLUGINS_UID_SEPARATION=on          # añade encima el muro independiente de uid
```

```bash title="4. Fijar un modo (avanzado)"
EVEY_PLUGINS_SANDBOX_MODE=landlock      # no arranca los plugins si Landlock no está disponible
```

```bash title="5. Un plugin falla con la separación de uid"
EVEY_PLUGINS_UID_SEPARATION=off
```

```bash title="6. Límites de recursos (modificables después desde Administración -> Plugins -> Límites de recursos)"
EVEY_PLUGINS_MEMORY_MB=256              # RAM máxima por plugin
EVEY_PLUGINS_PIDS_MAX=64                # número máximo de procesos e hilos
EVEY_PLUGINS_DISK_MAX_MB=100            # tamaño máximo de la carpeta de instalación (0 = sin límite)
EVEY_PLUGINS_CPU_NICE=10                # prioridad de CPU 0-19 (más alto = más cede a los demás)
```

## Cambiar el modo de sandbox en marcha

Cambia el **Modo aislado** en la tarjeta **Límites de recursos** de
**Administración -> Plugins**. Los plugins en marcha se reinician
automáticamente para que el nuevo modo se aplique enseguida, y el **Panel de
seguridad** muestra el modo activo. Un plugin que no puede funcionar con el
nuevo modo (por ejemplo `bubblewrap` dentro de un contenedor sin privilegios)
aparece con error: vuelve al modo anterior.

## Verificar que funciona

```bash
# el lanzador lleva la capacidad que usa la separación de uid
docker exec evey-server getcap /evey-plugin-launcher        # cap_setuid=ep

# con qué usuario se ejecuta un plugin en marcha (evey = compartido con el servidor,
# evey-plugin = separado); la imagen no tiene ps, así que pregunta a Docker
docker top evey-server -o user,pid,args | grep -iE "node|python"
```

El modo de sandbox activo es la insignia de **Administración -> Plugins ->
Panel de seguridad**.

<!-- sources
auto resolution and probes: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; runner_sandbox.go - effectiveMode, SandboxStatus, uidSepApplies, WithUIDSeparation, applyBasicSandbox
defaults: internal/config/config.go (EVEY_PLUGINS_SANDBOX_MODE, EVEY_PLUGINS_UID_SEPARATION, EVEY_PLUGINS_MEMORY_MB, _PIDS_MAX, _DISK_MAX_MB, _CPU_NICE)
restart on mode change: internal/app/plugin/runtime_service.go - RestartRunningPlugins
image: Dockerfile (setcap cap_setuid+ep /evey-plugin-launcher, no procps) ; docker-compose.yml (no security_opt)
labels: i18n/en/admin.yaml - plugins.security.title, probeLandlock, probeLauncher, probeBubblewrap, plugins.resourceLimits.title, resourceLimits.sandboxMode
-->
