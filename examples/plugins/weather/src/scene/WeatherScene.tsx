// Root scene component — provides SceneParams via React context.
import { createContext, useContext } from "react";
import type { SceneParams } from "../domain/weather";

export const SceneContext = createContext<SceneParams | null>(null);

export function useSceneParams(): SceneParams {
  const ctx = useContext(SceneContext);
  if (!ctx) throw new Error("useSceneParams must be used inside WeatherScene");
  return ctx;
}

export { SceneContext as WeatherSceneContext };
