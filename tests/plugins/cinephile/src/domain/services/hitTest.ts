// Pointer picking against the galaxy: given a point already expressed in the
// same normalized space as the node positions (the render adapter owns
// converting client/canvas pixels into that space), returns the index of the
// frontmost node under the pointer (smallest, since it's drawn on top when
// nodes overlap), or null.

export interface HitPoint {
  x: number;
  y: number;
}

export interface HitCircle {
  x: number;
  y: number;
  radius: number;
}

export function hitTestGalaxy(nodes: HitCircle[], point: HitPoint): number | null {
  let best: number | null = null;
  let bestRadius = Infinity;
  for (let i = 0; i < nodes.length; i++) {
    const node = nodes[i];
    const dx = point.x - node.x;
    const dy = point.y - node.y;
    if (dx * dx + dy * dy <= node.radius * node.radius && node.radius < bestRadius) {
      bestRadius = node.radius;
      best = i;
    }
  }
  return best;
}
