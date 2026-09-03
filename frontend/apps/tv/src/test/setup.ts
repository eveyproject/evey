// Screen tests render react-native primitives through per-file DOM mocks
// (`vi.mock("react-native", ...)`: View -> div, Pressable -> button, every
// prop forwarded). React then complains about props that only exist in
// react-native (`numberOfLines`, `accessible`, `hitSlop`, ...) and about
// HTML nesting rules a native tree does not have (a Pressable inside a
// Pressable is legal, a TextInput's handler is `onChangeText`). None of it
// describes the app - it never renders to the DOM outside these mocks - and
// it drowned the real warnings in `make pre-push`. Only these families are
// dropped; anything else still reaches the console.
const HARNESS_NOISE = [
  "React does not recognize the `%s` prop on a DOM element.",
  "Received `%s` for a non-boolean attribute `%s`.",
  "Unknown event handler property `%s`.",
  "You provided a `value` prop to a form field without an `onChange` handler.",
  "In HTML, %s cannot be a descendant of <%s>.",
  "<%s> cannot contain a nested %s.",
];

function isHarnessNoise(msg: unknown): boolean {
  if (typeof msg !== "string") return false;
  return HARNESS_NOISE.some((prefix) => msg.startsWith(prefix));
}

const realError = console.error.bind(console);
console.error = (...args: unknown[]) => {
  if (isHarnessNoise(args[0])) return;
  realError(...args);
};
