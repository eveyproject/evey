import { View, Text } from "react-native";
import { useTheme } from "../../../src/providers/ThemeProvider";

export default function HomeScreen() {
  const theme = useTheme();
  return (
    <View style={{ flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: theme.colors.bg }}>
      <Text style={{ color: theme.colors.text, fontSize: 18 }}>Home</Text>
    </View>
  );
}
