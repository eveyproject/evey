import { Component, type ReactNode } from "react";

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

  render() {
    if (!this.state.hasError) return this.props.children;

    return (
      <div
        style={{
          minHeight: "100vh",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          gap: "1.5rem",
          padding: "2rem",
          backgroundColor: "var(--bg, #111)",
          color: "var(--text, #ccc)",
          fontFamily: "system-ui, -apple-system, sans-serif",
        }}
      >
        <div
          style={{
            width: 64,
            height: 64,
            borderRadius: "50%",
            backgroundColor: "rgba(239, 68, 68, 0.15)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          <svg width="32" height="32" fill="none" stroke="#ef4444" strokeWidth={1.5} viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
          </svg>
        </div>
        <h1 style={{ fontSize: "1.5rem", fontWeight: 600, color: "var(--text, #fff)", margin: 0 }}>
          Something went wrong
        </h1>
        <p style={{ fontSize: "0.875rem", maxWidth: "28rem", textAlign: "center", color: "var(--text-muted, #999)", margin: 0, lineHeight: 1.6 }}>
          An unexpected error occurred. Please try reloading the page. If the problem persists, go back to the home page.
        </p>
        <div style={{ display: "flex", gap: "0.75rem", marginTop: "0.5rem" }}>
          <button
            onClick={() => window.location.reload()}
            style={{
              padding: "0.625rem 1.5rem",
              fontSize: "0.875rem",
              fontWeight: 500,
              borderRadius: "0.5rem",
              backgroundColor: "var(--accent, #6366f1)",
              color: "var(--overlay-text)",
              border: "none",
              cursor: "pointer",
            }}
          >
            Try Again
          </button>
          <button
            onClick={() => { window.location.href = "/home"; }}
            style={{
              padding: "0.625rem 1.5rem",
              fontSize: "0.875rem",
              fontWeight: 500,
              borderRadius: "0.5rem",
              backgroundColor: "transparent",
              color: "var(--text-muted, #999)",
              border: "1px solid var(--border, #333)",
              cursor: "pointer",
            }}
          >
            Go Home
          </button>
        </div>
      </div>
    );
  }
}
