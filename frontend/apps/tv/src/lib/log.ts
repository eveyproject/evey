// One logger for the TV app.
//
// Two problems it solves, both learned the hard way on a real device:
//
//  1. Finding anything. `adb logcat` mixes the app's console with Bluetooth
//     statistics, Glide chatter and cast-shell errors. Every line here carries
//     an "EVEY|<level>|<tag>|" prefix, which is what `make tv-logs` filters on
//     and colours - so the app's own story reads as one stream.
//  2. Knowing WHICH code is running. A whole debugging session was spent on a
//     device that was faithfully running an older bundle. bootMarker() prints
//     the commit the bundle was built from, and `make tv-state` compares it to
//     the working tree.
//
// Deliberately built on console.*: React Native routes those to logcat on
// Android and to the Xcode console on tvOS, so the same lines show up on both
// platforms with no native module and nothing to keep in sync.

import { setClientLogger } from "@evey/client";

export type LogLevel = "debug" | "info" | "warn" | "error";

// The tail kept in memory so a crash screen can show what led to it. Bounded:
// a TV app runs for hours and an unbounded buffer is a slow leak.
const RING_MAX = 200;
const ring: string[] = [];

function push(line: string): void {
  ring.push(line);
  if (ring.length > RING_MAX) ring.shift();
}

/** The last lines, oldest first - what the error screen offers to the viewer
 * and what a bug report should carry. */
export function recentLogs(): string[] {
  return [...ring];
}

function emit(level: LogLevel, tag: string, args: unknown[]): void {
  // Rendered once and passed as a single argument: console.* with several
  // arguments is split across lines by some transports, which breaks the
  // one-line-per-event contract `make tv-logs` relies on.
  const body = args
    .map((a) => {
      if (typeof a === "string") return a;
      if (a instanceof Error) return `${a.name}: ${a.message}`;
      try {
        return JSON.stringify(a);
      } catch {
        return String(a);
      }
    })
    .join(" ");
  const line = `EVEY|${level}|${tag}| ${body}`;
  push(line);
  switch (level) {
    case "error":
      console.error(line);
      break;
    case "warn":
      console.warn(line);
      break;
    default:
      console.log(line);
  }
}

export interface Logger {
  debug(...args: unknown[]): void;
  info(...args: unknown[]): void;
  warn(...args: unknown[]): void;
  error(...args: unknown[]): void;
}

/**
 * A logger for one area of the app. The tag is what makes a filtered read
 * useful: `make tv-logs player` shows the player's story and nothing else.
 */
export function logger(tag: string): Logger {
  return {
    debug: (...a) => emit("debug", tag, a),
    info: (...a) => emit("info", tag, a),
    warn: (...a) => emit("warn", tag, a),
    error: (...a) => emit("error", tag, a),
  };
}

/**
 * Announce which build is running, once, at startup.
 *
 * EXPO_PUBLIC_BUILD_SHA is injected by scripts/tv/up.sh when it starts Metro.
 * It is absent in a plain `expo start`, and "unknown" is an honest answer -
 * better than a stale value that would recreate the very confusion this is
 * here to prevent.
 */
export function bootMarker(): void {
  const sha = process.env.EXPO_PUBLIC_BUILD_SHA ?? "unknown";
  logger("boot").info(`boot sha=${sha} at=${new Date().toISOString()}`);
}

// The shared hooks (@evey/client) log through this logger too, so their lines
// carry the same prefix and land in the same filtered stream.
setClientLogger(logger);
