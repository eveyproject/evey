// Composition root: wire adapters into the application. This is the only
// file that knows concrete implementations.

import { WeatherApp } from "./application/WeatherApp";
import { OpenMeteoProvider } from "./adapters/openmeteo/OpenMeteoProvider";
import { EveyBridge } from "./adapters/evey/EveyBridge";
import { GlRenderer } from "./adapters/render/GlRenderer";

new WeatherApp({
  provider: new OpenMeteoProvider(),
  renderer: new GlRenderer(),
  bridge: new EveyBridge(),
}).start();
