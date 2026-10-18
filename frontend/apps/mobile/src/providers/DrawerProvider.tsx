import React, { createContext, useContext, useState, useCallback, useMemo, useRef, useEffect } from "react";
import { AccessibilityInfo, Animated, type View } from "react-native";

interface DrawerContextValue {
  isOpen: boolean;
  /** Pass the control that opened the drawer so a dismiss can hand the focus back to it. */
  open: (trigger?: View | null) => void;
  /** Close on the way to another screen: the focus belongs to that screen. */
  close: () => void;
  /** Close without navigating (Escape, Back, backdrop): the focus returns to the trigger. */
  dismiss: () => void;
  toggle: () => void;
  translateX: Animated.Value;
}

const DrawerContext = createContext<DrawerContextValue | null>(null);

export function DrawerProvider({ children }: { children: React.ReactNode }) {
  const [isOpen, setIsOpen] = useState(false);
  const translateX = useRef(new Animated.Value(-280)).current;
  const triggerRef = useRef<View | null>(null);

  // Stop any in-flight spring on unmount so a rapidly-mounted next provider
  // doesn't inherit half a transition or fire callbacks against a torn-down
  // tree.
  useEffect(() => {
    return () => translateX.stopAnimation();
  }, [translateX]);

  const open = useCallback((trigger?: View | null) => {
    triggerRef.current = trigger ?? null;
    setIsOpen(true);
    Animated.spring(translateX, {
      toValue: 0,
      useNativeDriver: true,
      tension: 80,
      friction: 10,
    }).start();
  }, [translateX]);

  const slideOut = useCallback((onClosed?: () => void) => {
    Animated.spring(translateX, {
      toValue: -280,
      useNativeDriver: true,
      tension: 80,
      friction: 10,
    }).start(() => {
      setIsOpen(false);
      onClosed?.();
    });
  }, [translateX]);

  const close = useCallback(() => slideOut(), [slideOut]);

  // The focus can only land on the trigger once the modal has gone: while it
  // is up, the screen reader cannot reach anything behind it.
  const dismiss = useCallback(() => {
    const trigger = triggerRef.current;
    slideOut(() => {
      if (trigger) AccessibilityInfo.sendAccessibilityEvent(trigger, "focus");
    });
  }, [slideOut]);

  const toggle = useCallback(() => {
    if (isOpen) close(); else open();
  }, [isOpen, open, close]);

  const value = useMemo(
    () => ({ isOpen, open, close, dismiss, toggle, translateX }),
    [isOpen, open, close, dismiss, toggle, translateX],
  );

  return (
    <DrawerContext.Provider value={value}>
      {children}
    </DrawerContext.Provider>
  );
}

export function useDrawer() {
  const ctx = useContext(DrawerContext);
  if (!ctx) throw new Error("useDrawer must be used within DrawerProvider");
  return ctx;
}
