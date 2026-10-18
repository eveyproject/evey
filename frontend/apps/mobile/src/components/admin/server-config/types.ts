import type { UseMutateAsyncFunction, UseMutateFunction } from "@tanstack/react-query";

export interface ConfigEntry {
  key: string;
  value: string;
}

// Matches the shape of the screen's existing saveConfigEntry mutation
// (PUT /api/v1/admin/server-config/:key), so it can be passed straight
// through to every card without each one re-declaring the mutation.
export type SaveEntry = UseMutateFunction<unknown, Error, { key: string; value: string }, unknown>;

// A card that writes several keys in one Save awaits each write and reads
// the outcome once, so it gets the promise form.
export type SaveEntryAsync = UseMutateAsyncFunction<unknown, Error, { key: string; value: string }, unknown>;
