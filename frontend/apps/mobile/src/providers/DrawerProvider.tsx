import { createContext, useContext, useState, useCallback, useRef, useEffect } from "react";
import { Animated } from "react-native";

interface DrawerContextValue {
  isOpen: boolean;
  open: () => void;
  close: () => void;
  toggle: () => void;
  translateX: Animated.Value;
}

const DrawerContext = createContext<DrawerContextValue | null>(null);

export function DrawerProvider({ children }: { children: React.ReactNode }) {
  const [isOpen, setIsOpen] = useState(false);
  const translateX = useRef(new Animated.Value(-280)).current;

  const open = useCallback(() => {
    setIsOpen(true);
    Animated.spring(translateX, {
      toValue: 0,
      useNativeDriver: true,
      tension: 80,
      friction: 10,
    }).start();
  }, [translateX]);

  const close = useCallback(() => {
    Animated.spring(translateX, {
      toValue: -280,
      useNativeDriver: true,
      tension: 80,
      friction: 10,
    }).start(() => setIsOpen(false));
  }, [translateX]);

  const toggle = useCallback(() => {
    if (isOpen) close(); else open();
  }, [isOpen, open, close]);

  return (
    <DrawerContext.Provider value={{ isOpen, open, close, toggle, translateX }}>
      {children}
    </DrawerContext.Provider>
  );
}

export function useDrawer() {
  const ctx = useContext(DrawerContext);
  if (!ctx) throw new Error("useDrawer must be used within DrawerProvider");
  return ctx;
}
