import { useCallback, useRef, useState } from "react";
import { eveyFetch } from "@evey/api";

interface StreamTokenResponse {
  token: string;
  mediaId: string;
}

export interface StreamTokenResult {
  streamToken: string | null;
  fetchStreamToken: (mediaId: string) => Promise<string>;
  isLoading: boolean;
}

export function useStreamToken(): StreamTokenResult {
  const [streamToken, setStreamToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const tokenRef = useRef<string | null>(null);

  const fetchStreamToken = useCallback(async (mediaId: string): Promise<string> => {
    setIsLoading(true);
    try {
      const res = await eveyFetch<StreamTokenResponse>({
        url: `/api/v1/media/${mediaId}/stream-token`,
        method: "POST",
      });
      tokenRef.current = res.token;
      setStreamToken(res.token);
      return res.token;
    } finally {
      setIsLoading(false);
    }
  }, []);

  return { streamToken, fetchStreamToken, isLoading };
}
