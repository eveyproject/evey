import { create } from "zustand";

type ToastType = "success" | "error" | "info" | "warning";

interface Toast {
  id: string;
  type: ToastType;
  message: string;
  duration?: number;
  /** Makes the toast body tappable (e.g. "download started" -> Downloads screen). */
  onPress?: () => void;
}

interface ToastState {
  toasts: Toast[];
  addToast: (toast: Omit<Toast, "id">) => string;
  removeToast: (id: string) => void;
}

let counter = 0;

export const useToastStore = create<ToastState>()((set) => ({
  toasts: [],
  addToast: (toast) => {
    const id = `toast-${++counter}`;
    set((state) => {
      const next = [...state.toasts, { ...toast, id }];
      // max 5 visible, drop oldest
      return { toasts: next.length > 5 ? next.slice(-5) : next };
    });
    return id;
  },
  removeToast: (id) =>
    set((state) => ({ toasts: state.toasts.filter((t) => t.id !== id) })),
}));

interface ToastOptions {
  onPress?: () => void;
  /** How long it stays, in ms (each display defaults to about 3 s). */
  duration?: number;
}

export const toast = {
  success: (message: string, opts?: ToastOptions) =>
    useToastStore.getState().addToast({ type: "success", message, ...opts }),
  error: (message: string, opts?: ToastOptions) =>
    useToastStore.getState().addToast({ type: "error", message, duration: 6000, ...opts }),
  info: (message: string, opts?: ToastOptions) =>
    useToastStore.getState().addToast({ type: "info", message, ...opts }),
  warning: (message: string, opts?: ToastOptions) =>
    useToastStore.getState().addToast({ type: "warning", message, ...opts }),
};

export type { Toast, ToastType };
