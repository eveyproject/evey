import { Component, type ReactNode } from "react";
import { TriangleAlert } from "lucide-react";
import i18n from "@/i18n";
import { isChunkLoadError, reloadOnceForChunkError } from "@/lib/chunkError";

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

  // `unknown`, not `Error`: what React hands over for a failed lazy chunk is
  // `undefined`, and reading `.name` off it threw inside this very handler - the
  // boundary then rendered nothing and its own TypeError became the message the
  // user saw ("can't access property "name", t is undefined"). Nothing in here
  // may assume a shape; chunkError.ts does the reading.
  componentDidCatch(error: unknown, errorInfo: React.ErrorInfo) {
    // Auto-reload once on a failed chunk load (network hiccup, or stale lazy
    // chunks still requested by a tab left open across a deploy).
    if (isChunkLoadError(error) && reloadOnceForChunkError()) return;
    // Logged in production too. The DEV-only branch this replaces meant a prod
    // crash left no trace anywhere - console, server, nothing - so the screen
    // above had to be traced back through the minified bundle.
    console.error("ErrorBoundary caught:", error, errorInfo?.componentStack);
  }

  render() {
    if (!this.state.hasError) return this.props.children;

    return (
      <div
        className="min-h-screen flex flex-col items-center justify-center gap-6 p-8 bg-[var(--bg)] text-[var(--text)] font-[var(--font-family,system-ui,-apple-system,sans-serif)]"
      >
        <div className="w-16 h-16 rounded-full bg-[var(--error-bg-subtle)] flex items-center justify-center">
          <TriangleAlert size={32} stroke="var(--danger-bg, #dc2626)" strokeWidth={1.5} />
        </div>
        <h1 className="text-2xl font-semibold text-[var(--text)] m-0">
          {i18n.t("common.errorBoundary.title")}
        </h1>
        <p className="text-sm max-w-md text-center text-[var(--text-muted)] m-0 leading-relaxed">
          {i18n.t("common.errorBoundary.description")}
        </p>
        <div className="flex gap-3 mt-2">
          <button
            onClick={() => window.location.reload()}
            aria-label={i18n.t("common.aria.reloadPage")}
            className="px-6 py-2.5 text-sm font-medium rounded-lg bg-[var(--accent)] text-[var(--overlay-text)] border-none cursor-pointer"
          >
            {i18n.t("common.errorBoundary.tryAgain")}
          </button>
          <button
            onClick={() => { window.location.replace("/"); }}
            aria-label={i18n.t("common.aria.goToHome")}
            className="px-6 py-2.5 text-sm font-medium rounded-lg bg-transparent text-[var(--text-muted)] border border-[var(--border)] cursor-pointer"
          >
            {i18n.t("common.errorBoundary.goHome")}
          </button>
        </div>
      </div>
    );
  }
}
