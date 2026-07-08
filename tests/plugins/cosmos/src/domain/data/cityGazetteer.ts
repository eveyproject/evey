// Embedded city -> coordinates lookup. Unlike weather-v2 (which resolves the
// user's city via a live geocoding API), this plugin declares NO
// external_fetch at all: every position it draws (stars, Moon, planets) is
// computed locally, so there is nothing to fetch over the network. A small
// gazetteer of major world cities keeps the "type your city" UX from
// weather-v2 while staying fully offline.

export interface CityCoord {
  name: string;
  lat: number;
  lon: number; // east positive
}

export const CITY_GAZETTEER: CityCoord[] = [
  { name: "Paris", lat: 48.8566, lon: 2.3522 },
  { name: "London", lat: 51.5072, lon: -0.1276 },
  { name: "Berlin", lat: 52.52, lon: 13.405 },
  { name: "Madrid", lat: 40.4168, lon: -3.7038 },
  { name: "Rome", lat: 41.9028, lon: 12.4964 },
  { name: "Lisbon", lat: 38.7223, lon: -9.1393 },
  { name: "Amsterdam", lat: 52.3676, lon: 4.9041 },
  { name: "Brussels", lat: 50.8503, lon: 4.3517 },
  { name: "Vienna", lat: 48.2082, lon: 16.3738 },
  { name: "Zurich", lat: 47.3769, lon: 8.5417 },
  { name: "Dublin", lat: 53.3498, lon: -6.2603 },
  { name: "Stockholm", lat: 59.3293, lon: 18.0686 },
  { name: "Oslo", lat: 59.9139, lon: 10.7522 },
  { name: "Copenhagen", lat: 55.6761, lon: 12.5683 },
  { name: "Helsinki", lat: 60.1699, lon: 24.9384 },
  { name: "Warsaw", lat: 52.2297, lon: 21.0122 },
  { name: "Prague", lat: 50.0755, lon: 14.4378 },
  { name: "Athens", lat: 37.9838, lon: 23.7275 },
  { name: "Moscow", lat: 55.7558, lon: 37.6173 },
  { name: "Istanbul", lat: 41.0082, lon: 28.9784 },
  { name: "New York", lat: 40.7128, lon: -74.006 },
  { name: "Los Angeles", lat: 34.0522, lon: -118.2437 },
  { name: "Chicago", lat: 41.8781, lon: -87.6298 },
  { name: "Toronto", lat: 43.6532, lon: -79.3832 },
  { name: "Montreal", lat: 45.5019, lon: -73.5674 },
  { name: "Mexico City", lat: 19.4326, lon: -99.1332 },
  { name: "Sao Paulo", lat: -23.5505, lon: -46.6333 },
  { name: "Rio de Janeiro", lat: -22.9068, lon: -43.1729 },
  { name: "Buenos Aires", lat: -34.6037, lon: -58.3816 },
  { name: "Santiago", lat: -33.4489, lon: -70.6693 },
  { name: "Bogota", lat: 4.711, lon: -74.0721 },
  { name: "Cairo", lat: 30.0444, lon: 31.2357 },
  { name: "Lagos", lat: 6.5244, lon: 3.3792 },
  { name: "Nairobi", lat: -1.2921, lon: 36.8219 },
  { name: "Johannesburg", lat: -26.2041, lon: 28.0473 },
  { name: "Cape Town", lat: -33.9249, lon: 18.4241 },
  { name: "Dubai", lat: 25.2048, lon: 55.2708 },
  { name: "Tel Aviv", lat: 32.0853, lon: 34.7818 },
  { name: "Riyadh", lat: 24.7136, lon: 46.6753 },
  { name: "Mumbai", lat: 19.076, lon: 72.8777 },
  { name: "New Delhi", lat: 28.6139, lon: 77.209 },
  { name: "Bangkok", lat: 13.7563, lon: 100.5018 },
  { name: "Singapore", lat: 1.3521, lon: 103.8198 },
  { name: "Jakarta", lat: -6.2088, lon: 106.8456 },
  { name: "Manila", lat: 14.5995, lon: 120.9842 },
  { name: "Hong Kong", lat: 22.3193, lon: 114.1694 },
  { name: "Shanghai", lat: 31.2304, lon: 121.4737 },
  { name: "Beijing", lat: 39.9042, lon: 116.4074 },
  { name: "Seoul", lat: 37.5665, lon: 126.978 },
  { name: "Tokyo", lat: 35.6762, lon: 139.6503 },
  { name: "Osaka", lat: 34.6937, lon: 135.5023 },
  { name: "Sydney", lat: -33.8688, lon: 151.2093 },
  { name: "Melbourne", lat: -37.8136, lon: 144.9631 },
  { name: "Auckland", lat: -36.8485, lon: 174.7633 },
  { name: "Reykjavik", lat: 64.1466, lon: -21.9426 },
];

const DEFAULT_CITY: CityCoord = CITY_GAZETTEER[0]; // Paris

function normalize(s: string): string {
  return s
    .trim()
    .toLowerCase()
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, ""); // strip combining diacritics
}

// Resolves a free-typed city name against the embedded gazetteer, entirely
// offline. Falls back to Paris (this plugin's documented default) when the
// name isn't found or is empty, rather than throwing.
export function resolveCity(cityName: string | null | undefined): CityCoord {
  if (!cityName || !cityName.trim()) return DEFAULT_CITY;
  const needle = normalize(cityName);
  const exact = CITY_GAZETTEER.find((c) => normalize(c.name) === needle);
  if (exact) return exact;
  const prefix = CITY_GAZETTEER.find((c) => normalize(c.name).startsWith(needle));
  return prefix ?? DEFAULT_CITY;
}
