// react-native-svg ships native source vite cannot parse; the player's scrims
// resolve here instead (same posture as react-native-webview.ts). Renders
// nothing: a gradient has no behavior to assert.
const Nothing = () => null;

export const Svg = Nothing;
export const Defs = Nothing;
export const LinearGradient = Nothing;
export const Stop = Nothing;
export const Rect = Nothing;
export default Svg;
