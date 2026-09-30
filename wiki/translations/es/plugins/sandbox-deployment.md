---
source_sha: a6f8003c3c1502fd56c9686aaf41877fd1dc643a86c7f3eb3cb6910f60615fd6
reviewed: false
---
# Sandbox de plugins: qué configuración para tu servidor

Evey ejecuta los plugins de terceros dentro de un sandbox. **Para casi todo el
mundo la respuesta es: deja los valores por defecto.** El servidor detecta tu
máquina y elige automáticamente el aislamiento más fuerte que puede. Sigue
leyendo solo si tienes un **NAS antiguo**, ejecutas Evey **sin Docker** o quieres
**endurecer aún más**.

Para el modelo técnico completo consulta [SECURITY.md](../SECURITY.md). Esta
página es la guía de decisión en lenguaje sencillo.

---

## Qué te protege (en palabras sencillas)

Un plugin que instalas queda encerrado. La caja, según tu máquina:

- **Limita qué archivos puede tocar**: ve su propia carpeta y las bibliotecas del
  sistema que necesita para funcionar, nada más (ni tus medios, ni tus secretos,
  ni los demás plugins).
- **Bloquea las acciones de sistema peligrosas**: depurar otros procesos, cargar
  módulos del kernel, cambiar la hora, montar discos, etc., se rechazan en el
  kernel.
- **Corta el acceso directo a la red**, siempre. Un plugin que pidió la red (y
  que tú aprobaste) solo llega a los hosts que declaró, a través del servidor.
- **Limita la CPU, la memoria, el disco y el número de procesos**, para que un
  plugin desbocado no pueda tumbar el servidor.
- **Mantiene tus secretos ilegibles**: la clave que firma los inicios de sesión y
  las copias de seguridad.

Además: **solo un administrador puede instalar** plugins, se **analizan** en
busca de código sospechoso, y tú **apruebas sus permisos** antes de que se
ejecuten.

**Comprueba lo que se aplica de verdad:** Admin → Plugins → Panel de seguridad
muestra el *sandbox del equipo*: el modo al que resolvió `auto` (`landlock`,
`bubblewrap` o `basic`) con los resultados de cada sonda. Si muestra **basic**, tu
equipo no ofrece aislamiento del sistema de archivos por ruta (los plugins se
ejecutan solo con bajada de UID + seccomp); las sondas te dicen por qué, y las
secciones de abajo cómo arreglarlo.

---

## Elige tu situación

| Tu servidor | Lo que tienes de serie | ¿Algo que hacer? |
|---|---|---|
| **Linux moderno en Docker** (kernel ≥ 5.13, prácticamente cualquiera desde ~2021): *el caso normal* | **El aislamiento más fuerte** (Landlock), automáticamente. | **Nada.** Deja los valores por defecto. |
| **NAS antiguo en Docker** (por ejemplo un Synology antiguo, kernel < 5.13) | Buen aislamiento, **menos los muros de archivos por carpeta**. Los secretos, la red y las acciones de sistema siguen bloqueados. | **Nada.** La separación de uid (abajo) se activa sola en este caso. |
| **Cualquier Linux, sin Docker** (sin contenedor) | **El aislamiento más fuerte**, automáticamente (Landlock en un kernel moderno, si no espacios de nombres bubblewrap). | Sigue la [guía sin contenedor](../deployment-bare-metal.md). |

Cómo saber cuál tienes: abre **Admin → Plugins**; la insignia del panel de
seguridad muestra el modo de sandbox realmente activo: `landlock` o `bubblewrap`
significa fuerte, `basic` significa el caso del "NAS antiguo".

---

## El único mando que quizá toques: la separación de uid

**Qué hace, en pocas palabras:** hace que un plugin se ejecute con una **cuenta
de usuario distinta** de la del propio Evey, así que físicamente no puede leer
los archivos secretos de Evey, aunque nada más lo impidiera.

**Cuándo importa:** solo en una **máquina antigua sin los muros de archivos por
carpeta** (es decir, el caso `basic`). En las máquinas modernas los muros de
archivos ya mantienen los secretos fuera de alcance, así que ahí la separación de
uid es solo una capa extra redundante.

**El ajuste**: `EVEY_PLUGINS_UID_SEPARATION`:

| Valor | Significado | Úsalo cuando |
|---|---|---|
| `auto` *(por defecto)* | Se activa **solo cuando de verdad hace falta** (el caso `basic`). Desactivada donde es redundante. | **Déjalo así**: hace lo correcto en cada máquina. |
| `on` | **Siempre activada**, incluso en máquinas modernas: un segundo muro independiente para los secretos, encima de los muros de archivos. | Quieres el máximo de precauciones y aceptas el pequeño coste de abajo. |
| `off` | **Nunca.** | Un plugin concreto se comporta mal con ella (raro). |

**La trampa:** la separación de uid es **incompatible con la opción de
endurecimiento `no-new-privileges` de Docker**: elige una. Si tu configuración no
puede concederla en una máquina antigua (`basic`), Evey **se niega a arrancar el
plugin** en lugar de ejecutarlo junto a tus secretos; en una máquina moderna
registra un aviso y ejecuta el plugin con normalidad (simplemente no tienes el
muro extra). La imagen oficial ya trae todo lo necesario; solo defines esta
variable.

**Pequeño coste de `on`:** el plugin se ejecuta con una cuenta aparte, así que un
plugin que escribe archivos temporales *dentro de su propia carpeta* podría
tropezar. Si pasa, pon `off`. (Por eso el valor por defecto es `auto`: solo
asume ese riesgo donde hay un beneficio real.)

---

## Los riesgos, en palabras sencillas

Lo que un plugin **malicioso** podría y no podría hacer, por escenario:

**Linux moderno (Docker o sin contenedor):**
Un plugin malo queda totalmente encerrado. **No puede** leer tus secretos, **no
puede** ver ni tocar los archivos de otros plugins, **no puede** hacer llamadas
de red arbitrarias y **no puede** alterar el sistema. La única salida realista es
un fallo grave del **kernel** de Linux, algo que vale para *cualquier* sandbox
que no sea una máquina virtual completa.

**NAS antiguo en Docker, sin muros por carpeta (`basic`):**
- ✅ Tus **secretos siguen protegidos** (clave de inicio de sesión, copias) por la
  cuenta de usuario aparte, que `auto` activa aquí.
- ✅ El plugin **no tiene red directa**; uno autorizado solo llega a los hosts que
  declaró.
- ✅ **Las acciones de sistema peligrosas siguen bloqueadas** en el kernel.
- ⚠️ Pero sin muros por carpeta, un plugin malo **podría leer archivos normales,
  no secretos**, **listar los procesos en ejecución** y, como los plugins
  comparten una cuenta, potencialmente **leer o alterar los archivos de otro
  plugin**.
- **En resumen:** en un equipo así instala solo plugins de confianza, o pasa a un
  kernel moderno para tener los muros de archivos completos.

---

## Ejemplos de configuración (copiar y pegar)

Van en tu `.env` (Docker compose) o como variables de entorno `EVEY_*`.

**1. Servidor moderno normal, recomendado (no hacer nada):**
```bash
# Los valores por defecto son correctos. Opcionalmente, hazlo explícito:
EVEY_PLUGINS_SANDBOX_MODE=auto          # elige el más fuerte disponible
# EVEY_PLUGINS_UID_SEPARATION=auto      # (por defecto) activa solo donde hace falta
```

**2. NAS antiguo:** nada que definir, `auto` ya ejecuta ahí los plugins con la
cuenta aparte. Deja `no-new-privileges` fuera del `security_opt` de tu compose,
porque lo impediría:
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=auto        # (por defecto) activa en el caso basic
```

**3. Máximo de precauciones en un servidor moderno (sumar ambos muros):**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto          # → landlock
EVEY_PLUGINS_UID_SEPARATION=on          # añade encima el muro de uid independiente
```

**4. Fijar un modo concreto (avanzado):**
```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock      # se niega a arrancar plugins si Landlock no está disponible (fallo en cerrado)
```

**5. Un plugin falla con la separación de uid:**
```bash
EVEY_PLUGINS_UID_SEPARATION=off
```

**6. Límites de recursos (todos editables después desde Admin → Plugins):**
```bash
EVEY_PLUGINS_MEMORY_MB=256              # RAM máxima por plugin
EVEY_PLUGINS_PIDS_MAX=64                # procesos/hilos máximos
EVEY_PLUGINS_DISK_MAX_MB=100            # tamaño máximo del directorio de instalación (0 = sin límite)
EVEY_PLUGINS_CPU_NICE=10                # prioridad de planificación 0-19 (más alto = más cortés)
```

---

## Cambiar el modo de sandbox en caliente

Puedes cambiar el modo desde la tarjeta **Límites de recursos** en **Admin →
Plugins**. Los plugins en ejecución **se reinician automáticamente** para que el
nuevo modo se aplique de inmediato (sin desactivar/activar a mano). La insignia de
seguridad muestra el modo realmente activo. Si un plugin no puede ejecutarse con
el nuevo modo (por ejemplo, elegiste `bubblewrap` en Docker, donde necesita
privilegios), aparece **en error** en el panel: basta con volver atrás.

---

## Comprobar que funciona

```bash
# el lanzador lleva la capacidad usada para la separación de uid:
docker exec evey-server getcap /evey-plugin-launcher        # → cap_setuid=ep

# con qué usuario se ejecuta un plugin (evey = compartido con el servidor, evey-plugin = separado):
docker exec evey-server ps -o user,pid,args | grep -v grep | grep -iE "node|python"

# el modo de sandbox realmente activo: la insignia de seguridad en la página Admin → Plugins.
```

---

## Ver también

- [SECURITY.md](../SECURITY.md): el modelo de seguridad completo de los plugins
  (cada capa, la tabla por modo, los límites conocidos).
- [deployment-bare-metal.md](../deployment-bare-metal.md): ejecutar sin Docker.
- [installation.md](../installation.md): el recorrido de autoalojamiento con Docker.
