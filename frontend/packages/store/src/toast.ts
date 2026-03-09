import { create } from "zustand";

type ToastType = "success" | "error" | "info" | "warning";

interface Toast {
  id: string;
  type: ToastType;
  message: string;
  duration?: number;
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

export const toast = {
  success: (message: string) =>
    useToastStore.getState().addToast({ type: "success", message }),
  error: (message: string) =>
    useToastStore.getState().addToast({ type: "error", message, duration: 6000 }),
  info: (message: string) =>
    useToastStore.getState().addToast({ type: "info", message }),
  warning: (message: string) =>
    useToastStore.getState().addToast({ type: "warning", message }),
};

export type { Toast, ToastType };
