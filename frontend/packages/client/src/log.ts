// The shared code logs through one sink so each app keeps its own transport:
// the TV app routes every line through its "EVEY|level|tag|" logger (what
// `make tv-logs` filters on), web and mobile are happy with the console. The
// default is the console, so nothing has to be wired for the lines to show.
export interface ClientLogger {
  debug(...args: unknown[]): void;
  info(...args: unknown[]): void;
  warn(...args: unknown[]): void;
  error(...args: unknown[]): void;
}

export type ClientLoggerFactory = (tag: string) => ClientLogger;

const consoleFactory: ClientLoggerFactory = (tag) => ({
  debug: (...a) => console.log(`[${tag}]`, ...a),
  info: (...a) => console.log(`[${tag}]`, ...a),
  warn: (...a) => console.warn(`[${tag}]`, ...a),
  error: (...a) => console.error(`[${tag}]`, ...a),
});

let factory: ClientLoggerFactory = consoleFactory;

/** Install the app's logger. Call once at startup, before any hook runs. */
export function setClientLogger(next: ClientLoggerFactory | null): void {
  factory = next ?? consoleFactory;
}

/**
 * A logger for one area of the shared code. Resolved at call time so a logger
 * created at module load still reaches the sink installed later.
 */
export function clientLogger(tag: string): ClientLogger {
  return {
    debug: (...a) => factory(tag).debug(...a),
    info: (...a) => factory(tag).info(...a),
    warn: (...a) => factory(tag).warn(...a),
    error: (...a) => factory(tag).error(...a),
  };
}
