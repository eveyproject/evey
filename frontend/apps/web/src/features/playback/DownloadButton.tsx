interface DownloadButtonProps {
  mediaId: string;
  title?: string;
}

export function DownloadButton({ mediaId, title }: DownloadButtonProps) {
  return (
    <a
      href={`/api/v1/media/${mediaId}/download`}
      download={title}
      className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]"
    >
      Download
    </a>
  );
}
