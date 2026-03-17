import type { ComponentType } from "react";

interface PluginWidget {
  pluginId: string;
  slot: string;
  component: ComponentType<Record<string, unknown>>;
}

const registry = new Map<string, PluginWidget>();
const slotIndex = new Map<string, Set<string>>();
const loadPromises = new Map<string, Promise<void>>();

function registryKey(pluginId: string, slot: string) {
  return `${pluginId}:${slot}`;
}

export function loadPlugin(pluginId: string, slot: string, bundleUrl: string): Promise<void> {
  const key = registryKey(pluginId, slot);
  if (registry.has(key)) return Promise.resolve();

  const existing = loadPromises.get(key);
  if (existing) return existing;

  const promise = import(/* @vite-ignore */ bundleUrl)
    .then((mod) => {
      const component = mod.default as ComponentType<Record<string, unknown>>;
      if (!component) return;
      registry.set(key, { pluginId, slot, component });
      const slots = slotIndex.get(slot) ?? new Set();
      slots.add(key);
      slotIndex.set(slot, slots);
    })
    .finally(() => {
      loadPromises.delete(key);
    });

  loadPromises.set(key, promise);
  return promise;
}

export function getPluginsForSlot(slotName: string): PluginWidget[] {
  const keys = slotIndex.get(slotName);
  if (!keys) return [];
  const result: PluginWidget[] = [];
  for (const key of keys) {
    const w = registry.get(key);
    if (w) result.push(w);
  }
  return result;
}

export function clearRegistry() {
  registry.clear();
  slotIndex.clear();
  loadPromises.clear();
}
