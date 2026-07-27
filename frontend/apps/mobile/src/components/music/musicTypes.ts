// Field names match the JSON the gRPC gateway emits - protojson serializes
// proto field `track_number` as `trackNumber`, `id` as `id`, etc. The web app
// maps these to its own PascalCase domain types in a select(); mobile reads
// the wire shape directly. Shared by the music screen and its playlists tab
// so both join server track data the same way.
export interface Track {
  id: string;
  title: string;
  artist: string;
  album: string;
  albumArtist?: string;
  duration: number;
  trackNumber: number;
  coverArtUrl?: string;
  genre?: string;
  year?: number;
}
