import { Pressable } from "react-native";
import { useDrawer } from "../../providers/DrawerProvider";
import { EveyMark } from "../EveyLogo";

// The brand lives with the menu, never on a page: closed it is the compact mark
// that opens the drawer, open it is the wordmark at the drawer's head - the
// same two states the web sidebar shows collapsed and expanded. Shared by the
// main (tabs) headers and every top-level admin screen.
export function BurgerButton() {
  const { toggle } = useDrawer();
  return (
    <Pressable onPress={toggle} style={{ marginLeft: 0, padding: 4 }} accessibilityRole="button" accessibilityLabel="Open menu">
      <EveyMark height={26} />
    </Pressable>
  );
}
