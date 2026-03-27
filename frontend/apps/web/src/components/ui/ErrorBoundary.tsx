import { Component, type ReactNode } from "react";
import { TriangleAlert } from "lucide-react";
import i18n from "@/i18n";

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(): State {
    return { hasError: true };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error("ErrorBoundary caught:", error, errorInfo);
  }

  render() {
    if (!this.state.hasError) return this.props.children;

    return (
      <div
        className="min-h-screen flex flex-col items-center justify-center gap-6 p-8 bg-[var(--bg,#111)] text-[var(--text,#ccc)] font-[var(--font-family,system-ui,-apple-system,sans-serif)]"
      >
        <div className="w-16 h-16 rounded-full bg-[var(--error-bg-subtle)] flex items-center justify-center">
          <TriangleAlert size={32} stroke="var(--color-danger, #ef4444)" strokeWidth={1.5} />
        </div>
        <h1 className="text-2xl font-semibold text-[var(--text,#fff)] m-0">
          {i18n.t("common.errorBoundary.title")}
        </h1>
        <p className="text-sm max-w-md text-center text-[var(--text-muted,#999)] m-0 leading-relaxed">
          {i18n.t("common.errorBoundary.description")}
        </p>
        <div className="flex gap-3 mt-2">
          <button
            onClick={() => window.location.reload()}
            aria-label={i18n.t("common.aria.reloadPage")}
            className="px-6 py-2.5 text-sm font-medium rounded-lg bg-[var(--accent,#6366f1)] text-[var(--overlay-text)] border-none cursor-pointer"
          >
            {i18n.t("common.errorBoundary.tryAgain")}
          </button>
          <button
            onClick={() => { window.location.replace("/"); }}
            aria-label={i18n.t("common.aria.goToHome")}
            className="px-6 py-2.5 text-sm font-medium rounded-lg bg-transparent text-[var(--text-muted,#999)] border border-[var(--border,#333)] cursor-pointer"
          >
            {i18n.t("common.errorBoundary.goHome")}
          </button>
        </div>
      </div>
    );
  }
}
