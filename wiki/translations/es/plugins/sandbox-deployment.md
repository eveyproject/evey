---
source_sha: 0846a653f87edb54addf042a0876b46fd3ab9e42930e3f12e27706e59ce0bc97
---
# Entorno aislado de extensiones - qué configuración para su servidor

Evey ejecuta las extensiones de terceros dentro de un entorno aislado. **Para
casi todo el mundo la respuesta es: deje los valores por defecto.** El servidor
detecta su máquina y elige automáticamente el aislamiento más fuerte que puede.
Siga leyendo solo si tiene un **NAS antiguo**, ejecuta Evey **sin Docker** o
quiere **apretar aún más**.

El modelo técnico completo está en [SECURITY.md](../SECURITY.md). Esta página es
la guía de decisión en palabras sencillas.

---

## Qué le protege (en palabras sencillas)

Una extensión que usted instala queda encerrada. Según su máquina, la caja:

- **Limita los archivos que puede tocar** - ve su propia carpeta y las
  bibliotecas del sistema que necesita para funcionar, nada más (ni sus medios,
  ni sus secretos, ni otras extensiones).
- **Bloquea las acciones de sistema peligrosas** - depurar otros procesos,
  cargar módulos del núcleo, cambiar la hora, montar discos, etc. se rechazan en
  el núcleo.
- **Corta la red** salvo que la extensión la haya pedido explícitamente (y usted
  haya aprobado ese permiso).
- **Limita CPU, memoria, disco y número de procesos**, para que una extensión
  desbocada no pueda tumbar el servidor.
- **Mantiene ilegibles sus secretos** - la clave que firma los inicios de sesión
  y las copias de seguridad.

Además: instalar extensiones está **reservado al administrador**, se **analizan**
en busca de código sospechoso, y usted **aprueba sus permisos** antes de que se
ejecuten.

**Compruebe lo que está realmente en vigor:** Admin → Plugins → Security
Dashboard muestra el *entorno aislado del host* - el modo al que se resolvió
`auto` (`landlock`, `bubblewrap` o `basic`) con el resultado de cada sonda. Si
muestra **basic**, su host no ofrece aislamiento del sistema de archivos por
ruta (las extensiones se ejecutan solo con cambio de UID + seccomp); las sondas
le dicen por qué, y las secciones de abajo cómo arreglarlo.

---

## Elija su situación

| Su servidor | Lo que obtiene sin hacer nada | ¿Algo que hacer? |
|---|---|---|
| **Linux moderno en Docker** (núcleo ≥ 5.13 - prácticamente todo desde ~2021) - *el caso normal* | **El aislamiento más fuerte** (Landlock), automáticamente. | **Nada.** Deje los valores por defecto. |
| **NAS antiguo en Docker** (p. ej. un Synology antiguo, núcleo < 5.13) | Buen aislamiento, **sin los muros de archivos por carpeta**. Secretos, red y acciones del sistema siguen bloqueados. | Opcional: active la **separación de UID** (abajo) como doble protección de los secretos. |
| **Cualquier Linux, sin Docker** (bare metal) | **El aislamiento más fuerte**, automáticamente (Landlock en un núcleo moderno, si no espacios de nombres de bubblewrap). | Siga la [guía sin contenedor](../deployment-bare-metal.md). |

Cómo saber cuál le ha tocado: abra **Admin → Plugins** - la insignia del panel
de seguridad muestra el modo de aislamiento realmente en vigor: `landlock` o
`bubblewrap` significa fuerte, `basic` significa el caso «NAS antiguo».

---

## El único mando que quizá toque: la separación de UID

**Qué hace, dicho llanamente:** hace que una extensión se ejecute con una
**cuenta de usuario distinta** de la de Evey, de modo que físicamente no puede
leer los archivos secretos de Evey - aunque nada más la detuviera.

**Cuándo importa:** solo en una **máquina antigua sin los muros de archivos por
carpeta** (es decir, el caso `basic`). En máquinas modernas los muros de
archivos ya dejan los secretos fuera de su alcance, así que ahí la separación de
UID es solo una capa extra redundante.

**El ajuste** - `EVEY_PLUGINS_UID_SEPARATION`:

| Valor | Significado | Úselo cuando |
|---|---|---|
| `auto` *(por defecto)* | Se activa **solo cuando de verdad hace falta** (el caso `basic`). Desactivado donde es redundante. | **Déjelo así** - hace lo correcto en cada máquina. |
| `on` | **Siempre activo**, también en máquinas modernas - un segundo muro independiente alrededor de los secretos, además de los muros de archivos. | Quiere la máxima doble protección y acepta el pequeño coste de abajo. |
| `off` | **Nunca.** | Una extensión concreta se comporta mal con él (raro). |

**La pega:** la separación de UID es **incompatible con la opción de
endurecimiento de Docker `no-new-privileges`** - elija una. Si su configuración
no puede concederla en una máquina antigua (`basic`), Evey **se niega a arrancar
la extensión** en lugar de ejecutarla junto a sus secretos; en una máquina
moderna registra un aviso y ejecuta la extensión con normalidad (simplemente no
obtiene el muro extra). La imagen oficial ya trae todo lo necesario; usted solo
fija esta variable.

**Pequeño coste de `on`:** la extensión se ejecuta con una cuenta aparte, así
que una extensión que escriba archivos temporales *dentro de su propia carpeta*
podría tropezar. Si pasa, ponga `off`. (Por eso el valor por defecto es `auto`:
solo asume ese riesgo donde hay un beneficio real.)

---

## Los riesgos, en palabras sencillas

Lo que una extensión **maliciosa** podría y no podría hacer - según el escenario:

**Linux moderno (Docker o bare metal):**
Una extensión mala queda completamente encerrada. **No puede** leer sus
secretos, **no puede** ver ni tocar los archivos de otras extensiones, **no
puede** hacer llamadas de red arbitrarias y **no puede** manipular el sistema.
La única salida realista es un fallo grave del **núcleo** Linux - algo que vale
para *cualquier* entorno aislado que no sea una máquina virtual completa.

**NAS antiguo en Docker, sin muros por carpeta (`basic`):**
- ✅ Sus **secretos siguen protegidos** (clave de inicio de sesión, copias de
  seguridad) - *sobre todo si pone la separación de UID en `on`*.
- ✅ La extensión **no tiene red** salvo que se le permita.
- ✅ **Las acciones de sistema peligrosas siguen bloqueadas** en el núcleo.
- ⚠️ Pero sin muros por carpeta, una extensión mala **podría leer archivos
  corrientes no secretos**, **listar los procesos en marcha** y - como las
  extensiones comparten una cuenta - posiblemente **leer o perturbar los
  archivos de otra extensión**.
- **En resumen:** instale en un host así solo extensiones de confianza, o pase a
  un núcleo moderno para tener los muros de archivos completos.

---

## Ejemplos de configuración (para copiar)

Van en su `.env` (Docker compose) o como variables de entorno `EVEY_*`.

**1. Servidor moderno normal - recomendado (no hacer nada):**
```bash
# Defaults are correct. Optionally make it explicit:
EVEY_PLUGINS_SANDBOX_MODE=auto          # picks the strongest available
# EVEY_PLUGINS_UID_SEPARATION=auto      # (default) on only where needed
```

**2. NAS antiguo, máxima protección de los secretos:**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=on          # force the secret wall on
# and do NOT set no-new-privileges in your compose security_opt
```

**3. Máxima doble protección en un servidor moderno (los dos muros):**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto          # → landlock
EVEY_PLUGINS_UID_SEPARATION=on          # add the independent uid wall on top
```

**4. Fijar un modo concreto (avanzado):**
```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock      # refuse to start plugins if Landlock is unavailable (fail‑closed)
```

**5. Una extensión falla con la separación de UID:**
```bash
EVEY_PLUGINS_UID_SEPARATION=off
```

**6. Límites de recursos (todos editables después desde Admin → Plugins):**
```bash
EVEY_PLUGINS_MEMORY_MB=256              # max RAM per plugin
EVEY_PLUGINS_PIDS_MAX=64                # max processes/threads
EVEY_PLUGINS_DISK_MAX_MB=100            # max install‑dir size (0 = off)
EVEY_PLUGINS_CPU_NICE=10                # scheduling priority 0-19 (higher = nicer)
```

---

## Cambiar el modo de aislamiento en marcha

Puede cambiar el modo desde la tarjeta **Resource limits** de **Admin → Plugins**.
Las extensiones en marcha se **reinician automáticamente** para que el nuevo modo
se aplique de inmediato (sin desactivar/activar a mano). La insignia de seguridad
muestra el modo realmente en vigor. Si una extensión no puede ejecutarse con el
nuevo modo (p. ej. si eligió `bubblewrap` en Docker, donde necesita privilegios),
aparece como **en error** en el panel - basta con volver atrás.

---

## Comprobar que funciona

```bash
# the launcher carries the capability used for uid separation:
docker exec evey-server getcap /evey-plugin-launcher        # → cap_setuid=ep

# which user a running plugin runs as (evey = shared with server, evey-plugin = separated):
docker exec evey-server ps -o user,pid,args | grep -v grep | grep -iE "node|python"

# the sandbox mode actually in force: the Security badge on the Admin → Plugins page.
```

---

## Vea también

- [SECURITY.md](../SECURITY.md) - el modelo de seguridad completo de las
  extensiones (cada capa, la tabla por modo, los límites conocidos).
- [deployment-bare-metal.md](../deployment-bare-metal.md) - funcionar sin Docker.
- [installation.md](../installation.md) - la guía de autoalojamiento con Docker.
