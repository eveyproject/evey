// Composition root: wire adapters into the application. This is the only
// file that knows concrete implementations.

import { CinephileApp } from "./application/CinephileApp";
import { EveyBridge } from "./adapters/evey/EveyBridge";
import { CanvasRenderer } from "./adapters/render/CanvasRenderer";

new CinephileApp({
  renderer: new CanvasRenderer(),
  bridge: new EveyBridge(),
}).start();
