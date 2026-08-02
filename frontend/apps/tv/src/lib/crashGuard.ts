import { logger, recentLogs } from "./log";

// Last line of defence: an error that escapes every React boundary.
//
// A television is not a phone. Nobody is holding it, there is no app switcher
// reflex, and the room is three metres away - so a process that dies takes the
// evening with it. The rule this file enforces is that a JavaScript error can
// degrade the app but never end it.
//
// It deliberately does NOT replace error boundaries. A boundary keeps the rest
// of the UI alive and offers a way back; this only catches what no boundary
// saw: a throw from a timer, an event handler, or a promise nobody awaited.
//
// Works the same on tvOS: ErrorUtils and the promise hooks are React Native's,
// not Android's.

const log = logger("crash");

interface ErrorUtilsLike {
  getGlobalHandler?: () => ((error: unknown, isFatal?: boolean) => void) | undefined;
  setGlobalHandler?: (handler: (error: unknown, isFatal?: boolean) => void) => void;
}

function describe(error: unknown): string {
  if (error instanceof Error) return `${error.name}: ${error.message}\n${error.stack ?? ""}`;
  return String(error);
}

let installed = false;

/**
 * Install the global handlers. Idempotent, and safe to call before anything
 * else in the app - it takes no dependency on navigation, theme or i18n, all
 * of which may be the very thing that failed.
 */
export function installCrashGuard(): void {
  if (installed) return;
  installed = true;

  const errorUtils = (globalThis as { ErrorUtils?: ErrorUtilsLike }).ErrorUtils;
  const previous = errorUtils?.getGlobalHandler?.();

  errorUtils?.setGlobalHandler?.((error, isFatal) => {
    log.error(`unhandled${isFatal ? " (fatal)" : ""}: ${describe(error)}`);

    // In development the red box is worth more than a graceful degradation:
    // it puts the stack in front of whoever is looking. In a shipped build the
    // same call would tear the app down, which is exactly what we refuse.
    if (__DEV__ && previous) {
      previous(error, isFatal);
      return;
    }
    // Swallowed on purpose. The screen the viewer was on keeps running; if the
    // failure did break the UI, the nearest error boundary has already shown
    // its own recovery screen.
  });

  // Promise rejections nobody handled. React Native only warns about these,
  // and the warning is invisible on a television - but they are the most
  // common shape of "the screen just stopped updating".
  const withHandler = globalThis as { onunhandledrejection?: (e: unknown) => void };
  const previousRejection = withHandler.onunhandledrejection;
  withHandler.onunhandledrejection = (event: unknown) => {
    const reason = (event as { reason?: unknown })?.reason ?? event;
    log.error(`unhandled rejection: ${describe(reason)}`);
    previousRejection?.(event);
  };

  log.info("crash guard installé");
}

/**
 * The tail of the log, for a crash screen to show. Kept here so a screen never
 * has to reach into the logger's internals.
 */
export function crashContext(): string {
  return recentLogs().slice(-20).join("\n");
}
