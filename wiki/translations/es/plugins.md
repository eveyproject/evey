---
source_sha: f58b5dccd7a97d42b2fc6bf6a3137cd005cca7f7658e3dc3f906106cce0d7132
reviewed: true
---
# Plugins { #plugins }

Esta página es para el administrador: cómo instalar un plugin o un tema, qué
aceptas al hacerlo y qué le deja tocar el servidor en tu equipo. Todo pasa en
**Administración → Plugins**; las páginas para quienes escriben plugins están
listadas [al final](#writing-a-plugin).

Un plugin añade a Evey lo que no hace por sí mismo: una sección en la página
de inicio, una capa encima del reproductor, una fuente de metadatos, un tema.
Cada plugin es un proceso aparte que se comunica con el servidor a través de
un socket local, aislado en un sandbox y limitado a los permisos que declara su
manifiesto. Su interfaz, si tiene una, funciona en un marco aislado de la
página.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![La página de inicio sin plugins](images/home-no-plugin.webp){ loading=lazy }
  <figcaption>Sin plugins.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![La página de inicio con el plugin del tiempo](images/home.webp){ loading=lazy }
  <figcaption>Con el plugin de ejemplo del tiempo: una sección arriba, dibujada en su propio marco.</figcaption>
</figure>

</div>

## Instalar un plugin { #install-a-plugin }

1. En **Administración → Plugins**, abre **Instalar plugin** y elige el
   **Archivo de plugin (.zip)**. Si el autor proporciona una firma, añádela en
   **Archivo de firma (.zip.sig, opcional)**. La pestaña **Desde URL** instala
   a partir de un enlace; añade `#sha256:<hash>` a la URL para que se
   compruebe la descarga.
2. **Vista previa e instalar** analiza el paquete sin instalarlo. La vista
   previa muestra los **Permisos** y las **Capacidades** que pide el plugin,
   y el resultado del **Análisis estático**. Los huecos (slots), los
   servidores y la firma se muestran en la actualización y en el **Panel de
   seguridad**.
3. Lee esa lista como un contrato: es lo único que el plugin podrá hacer.
   Confirma y el plugin arranca.

<figure class="evey-shot" markdown="span">
  ![Administración → Plugins en el servidor de demostración](images/admin-plugins.webp){ loading=lazy }
  <figcaption><strong>Administración → Plugins</strong>: los entornos de ejecución disponibles, los límites de recursos y una tarjeta por plugin (interfaz en inglés).</figcaption>
</figure>

Un paquete cuyo análisis encuentra un patrón crítico se rechaza, respondas lo
que respondas. Los avisos se muestran y la decisión es tuya. El lenguaje del
plugin tiene que estar entre los **Entornos de ejecución disponibles** del
servidor: la imagen por defecto trae Python, Node.js, PHP, Ruby y Lua, la imagen
`-nodejs` conserva Python y Node.js y la imagen `-lean` solo Python.

!!! tip "Plugins firmados"
    El autor de un plugin puede firmar el ZIP con [cosign](https://docs.sigstore.dev/cosign/installation/),
    y tú decides qué pasa con un plugin sin firma o mal firmado. Para
    comprobar las firmas, dale al servidor la clave pública del autor y elige
    una política:

    ```sh
    EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
    EVEY_PLUGINS_COSIGN_POLICY=warn      # off | warn (por defecto) | enforce
    ```

    Con `warn`, una firma que falla se muestra en la vista previa y en el
    **Panel de seguridad**; con `enforce`, la instalación o la actualización
    se rechaza. Sin clave pública no se verifica nada, y el servidor no
    necesita el binario `cosign`. Para los autores: consulta
    [Firma tu plugin](plugins/dev-setup.md#7-sign-your-plugin).

## Actualizar un plugin { #update-a-plugin }

En la tarjeta del plugin, **Actualizar desde origen…** acepta el nuevo ZIP
(una ruta o una URL) y muestra la misma vista previa. Una actualización tiene
que llevar una versión más alta que la instalada. Cuando pide nuevos
permisos, capacidades, slots o servidores, la vista previa dice **Esta
actualización solicita nuevos permisos** y muestra la diferencia: no se
aplica nada hasta que eliges **Aprobar e instalar**. El paquete analizado es
el paquete instalado: su hash se vuelve a comprobar cuando se aplica la
actualización.

## Gestionar los plugins instalados { #manage-the-installed-plugins }

Cada tarjeta muestra el estado del plugin (**Activado** o **Desactivado**) y
ofrece:

| Acción | Qué hace |
| --- | --- |
| **Activado** / **Desactivado** | Arranca o detiene el proceso del plugin. Un plugin desactivado conserva sus archivos y su configuración. |
| **Configuración** | Los ajustes que declara el plugin (una clave de API, una ciudad, una unidad...). Los campos secretos se muestran ocultos y nunca se devuelven al navegador. |
| **Registros** | Las últimas líneas que el plugin escribió a través del servidor. |
| **Desinstalar** | Elimina el proceso, los archivos y los datos guardados del plugin. |

**Límites de recursos**, arriba de la página, pone un tope a cada proceso de
plugin. Los valores del panel son los valores por defecto; la variable de
entorno los define mientras el panel no se haya guardado nunca.

| Campo | Por defecto | Variable de entorno |
| --- | --- | --- |
| **Memoria máx. (MiB)** | 256 | `EVEY_PLUGINS_MEMORY_MB` |
| **Procesos máx.** | 64 | `EVEY_PLUGINS_PIDS_MAX` |
| **Disco máx. (MiB)** | 100 | `EVEY_PLUGINS_DISK_MAX_MB` |
| **Nice de CPU (0-19)** | 10 | `EVEY_PLUGINS_CPU_NICE` |
| **Modo de sandbox** | `auto` | `EVEY_PLUGINS_SANDBOX_MODE` |

La memoria, los procesos y el disco se aplican en la siguiente revisión del
monitor, que detiene un plugin que los supere; la prioridad de CPU y el modo
de sandbox se aplican a los plugins que arranquen después. `0` restablece el
valor por defecto para la memoria y los procesos; solo el límite de disco trata `0` como desactivado.

El **Panel de seguridad** guarda, para cada plugin, lo que encontró el
análisis, el resultado de la firma y el último error. Su línea **Sandbox del
host** muestra el modo de sandbox realmente activo en esta máquina: si dice
`basic`, el equipo no ofrece aislamiento del sistema de archivos por rutas, y
[Configurar el sandbox de plugins](plugins/sandbox-deployment.md) explica qué
cambiar.

Los plugins están activados por defecto. `EVEY_PLUGINS_ENABLED=false`
desactiva todo el sistema de plugins.

## Lo que un plugin puede hacer en tu equipo { #what-a-plugin-can-do-on-your-host }

Diga lo que diga su manifiesto, un plugin:

- ve su propia carpeta, no los datos del servidor ni los archivos de medios;
- no tiene acceso directo a la red: un plugin con el permiso `network` solo
  llega a los servidores que declaró, a través del servidor de Evey, con `GET`
  y `HEAD`;
- no puede lanzar procesos hijos salvo que declare `subprocess`;
- funciona con los límites de recursos de arriba y, si el equipo lo permite,
  con un usuario aparte.

Su interfaz queda limitada a los slots que declara el manifiesto: una
sección o la zona destacada de la página de inicio, un panel lateral en una
biblioteca o en una página de detalle, una capa encima del reproductor, los
ajustes de apariencia, la parte de abajo de la barra lateral de navegación, o
una entrada en el menú de administración que abre la página propia del
plugin. Un plugin nunca puede dibujar en las pantallas de inicio de sesión,
de configuración inicial, de seguridad ni en las pantallas de administración
existentes. En su marco, no tiene acceso a la página, ni a tus cookies, ni a
tu sesión: lo que lee del servidor pasa por un token de corta duración,
limitado a los dos permisos que puede pedir (`public` para una vista segura
de los títulos y carteles de los medios, `prefs` para las preferencias del
usuario actual en su propio espacio de nombres).

Los permisos que puede pedir un plugin están listados en
[Permisos y slots](plugins/security-model.md). Cómo funciona el sandbox, y lo
que todavía no puede impedir, está en
[Sandbox de plugins y modelo de amenazas](SECURITY.md).

!!! warning "El servidor controla con quién se comunica un plugin, no lo que envía"
    En cuanto un servidor está en la lista de un plugin, Evey no puede
    inspeccionar lo que el plugin le envía. Revisa un plugin de terceros
    antes de instalarlo, como harías con cualquier programa, y considera su
    lista de servidores como la lista de quienes pueden recibir los títulos
    de tu biblioteca.

## Temas { #themes }

Un tema es un paquete de tipo `kind: theme` sin proceso: una hoja de estilos
y, opcionalmente, un script aislado en un marco oculto. Se instala desde el
mismo cuadro **Instalar plugin**; la hoja de estilos se sanea al instalarla.
Después, cada usuario lo elige en **Ajustes → Apariencia → Tema**. Lo que
puede contener un tema se describe en [Temas](themes.md).

## Escribir un plugin { #writing-a-plugin }

| Página | Contenido |
| --- | --- |
| [Escribir un plugin](plugins/dev-setup.md) | Un primer plugin paso a paso, el ciclo de instalación, la firma y la distribución. Empieza por aquí. |
| [Referencia de la API de plugins](plugins/api-reference.md) | El manifiesto, las llamadas del backend y el almacén de datos, el puente `evey.*` de la interfaz. |
| [Permisos y slots](plugins/security-model.md) | Lo que puede declarar un manifiesto, lo que se comprueba al instalar, las reglas de las peticiones externas, los slots. |
| [Temas](themes.md) | Cambiar el estilo de la interfaz con CSS y un script en un sandbox. |
| [Webhooks y claves de API](webhooks-api-keys.md) | Integrar Evey con otras herramientas sin escribir un plugin. |

<!-- sources
install dialog, from URL with #sha256: i18n admin.plugins.installPlugin, pluginFileZip, sigFileOptional, fromUrl, installFromUrlHint, previewInstall, staticAnalysis ; internal/app/plugin/install.go - downloadIfURL
critical findings refuse, warnings: internal/app/plugin/install.go - validateStagedPlugin, checkEscalatedWarnings ; i18n admin.plugins.blockedByAnalysis, installAnyway
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go - runtimeBinaries ; i18n admin.plugins.availableRuntimes
cosign policy: internal/config/config.go - CosignPolicy, CosignPublicKey ; internal/adapters/driven/pluginrunner/cosign.go
update, version higher, escalation, hash re-checked: internal/app/plugin/install.go - resolveExistingPlugin ; internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/permissions.go - DiffPermissions ; i18n admin.plugin.update, admin.plugins.permissionEscalation.*
card actions: i18n admin.plugin.enabled, disabled, config, logs, uninstall ; secrets masked: internal/domain/plugin/model.go - IsSecretConfigField ; internal/adapters/driving/grpcapi/admin_plugin.go
resource limits defaults and env: internal/config/config.go - Plugins (MemoryMB, PIDsMax, DiskMaxMB, CPUNice, SandboxMode) ; i18n admin.plugins.resourceLimits.*
security dashboard, host sandbox: i18n admin.plugins.security.* ; internal/app/plugin/security_dashboard_service.go
plugins enabled flag: internal/config/config.go - PluginsEnabled
fetch broker GET/HEAD: internal/adapters/driving/pluginhost/fetch.go - Fetch
subprocess: docs/plugins/security-model.md (Permissions catalog)
slots: internal/domain/plugin/slots.go - AllowedSlots
scoped token, public/prefs: docs/adr/0012-plugin-token-deny-by-default.md ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes
theme selection: frontend/apps/web/src/features/user/ThemeSettings.tsx ; i18n common.appearance, common.theme
-->
