import { Component, type ReactNode } from "react";

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null };

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
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
          gap: "1rem",
          padding: "2rem",
          backgroundColor: "#111",
          color: "#ccc",
        }}
      >
        <h1 style={{ fontSize: "1.5rem", fontWeight: 600, color: "#fff" }}>
          Something went wrong
        </h1>
        <p style={{ fontSize: "0.875rem", maxWidth: "32rem", textAlign: "center" }}>
          {this.state.error?.message ?? "An unexpected error occurred."}
        </p>
        <button
          onClick={() => window.location.reload()}
          style={{
            marginTop: "0.5rem",
            padding: "0.5rem 1.5rem",
            fontSize: "0.875rem",
            fontWeight: 500,
            borderRadius: "0.375rem",
            backgroundColor: "#6366f1",
            color: "#fff",
            border: "none",
            cursor: "pointer",
          }}
        >
          Reload
        </button>
      </div>
    );
  }
}
