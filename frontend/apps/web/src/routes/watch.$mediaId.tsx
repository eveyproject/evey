import { createFileRoute } from "@tanstack/react-router";

// Route definition ONLY - the guest player itself lives in the .lazy sibling.
// Keeping this file free of component imports keeps HLSPlayer (hls.js, ~520 KB)
// out of the entry bundle: a non-lazy route file is imported statically by
// routeTree.gen.ts, so anything it pulls in ships to every page, home included.
export const Route = createFileRoute("/watch/$mediaId")({});
