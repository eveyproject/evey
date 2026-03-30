export function artworkUrl(serverUrl: string, mediaId: string, type: "primary" | "backdrop" = "primary") {
  return `${serverUrl}/api/v1/media/${mediaId}/artwork/${type}`;
}
