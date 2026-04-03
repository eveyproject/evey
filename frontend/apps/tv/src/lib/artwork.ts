export function artworkUrl(serverUrl: string, mediaId: string, type: "poster" | "backdrop" = "poster") {
  return `${serverUrl}/api/v1/media/${mediaId}/artwork/${type}`;
}
