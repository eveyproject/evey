import { useAuthStore } from "@evey/store";

interface DownloadButtonProps {
  mediaId: string;
  title?: string;
}

export function DownloadButton({ mediaId, title }: DownloadButtonProps) {
  const token = useAuthStore((s) => s.accessToken);
  const url = `/api/v1/media/${mediaId}/download${token ? `?token=${encodeURIComponent(token)}` : ""}`;

  return (
    <a
      href={url}
      download={title}
      data-testid="download-btn"
      className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]"
    >
      <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v12m0 0l-4-4m4 4l4-4M4 18h16" />
      </svg>
      Download
    </a>
  );
}
