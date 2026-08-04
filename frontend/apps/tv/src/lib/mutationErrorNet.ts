import { MutationCache } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { logger } from "./log";
import { mutationErrorMessage } from "./mutationErrorMessage";

const log = logger("mutation");

/**
 * A write that fails must never pass for a write that worked. An audit
 * across the three apps found dozens of screens calling mutate()/mutateAsync
 * with no onError: every playback setting on this screen (settings/playback)
 * did not stick on failure and nothing said so.
 *
 * A mutation that reports its own failures keeps doing so - the net would
 * only double the message. One with a reason to fail quietly says
 * meta.silentError. Mirrors apps/web/src/lib/mutationErrorNet.ts.
 */
export function mutationErrorNet(): MutationCache {
  return new MutationCache({
    onError: (error, _vars, _ctx, mutation) => {
      if (mutation.options.onError || mutation.meta?.silentError) return;
      log.warn("mutation failed", error);
      toast.error(mutationErrorMessage(error));
    },
  });
}
