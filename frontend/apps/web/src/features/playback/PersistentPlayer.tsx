import { usePlayerStore } from "@evey/store";
import { MiniPlayer } from "./MiniPlayer";

export function PersistentPlayer() {
  const { isActive, mode } = usePlayerStore();

  if (!isActive || mode !== "mini") return null;

  return <MiniPlayer />;
}
