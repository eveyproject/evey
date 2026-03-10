import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@evey/store";

export interface Chapter {
  title: string;
  startTimeMs: number;
  endTimeMs: number;
}

export function useChapters(mediaId: string | undefined) {
  const token = useAuthStore((s) => s.accessToken);
  return useQuery<Chapter[]>({
    queryKey: ["/api/v1/media/chapters", mediaId],
    queryFn: async () => {
      const res = await fetch(`/api/v1/media/${mediaId}/chapters`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!res.ok) return [];
      const data = await res.json();
      return data.chapters ?? [];
    },
    enabled: !!mediaId,
    staleTime: Infinity,
  });
}
