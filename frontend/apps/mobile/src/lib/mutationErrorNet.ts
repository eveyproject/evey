import { MutationCache } from "@tanstack/react-query";
import { toast } from "@evey/store";
import { mutationErrorMessage } from "./mutationErrorMessage";

/**
 * A write that fails must never pass for a write that worked. An audit
 * across the three apps found dozens of screens calling mutate()/mutateAsync
 * with no onError (or an empty catch): the row did not move, the setting did
 * not stick, and nothing on screen said so.
 *
 * A mutation that reports its own failures keeps doing so - the net would
 * only double the message. One with a reason to fail quietly says
 * meta.silentError. Mirrors apps/web/src/lib/mutationErrorNet.ts.
 */
export function mutationErrorNet(): MutationCache {
  return new MutationCache({
    onError: (error, _vars, _ctx, mutation) => {
      if (mutation.options.onError || mutation.meta?.silentError) return;
      console.warn("mutation failed", error);
      toast.error(mutationErrorMessage(error));
    },
  });
}
