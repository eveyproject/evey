import { defineConfig } from "orval";

const swaggerDir = "../../gen/evey/v1";

const services = [
  "library", "user", "playback", "transcoding", "mkv",
  "metadata", "admin", "activity", "notification", "feed", "webhook",
];

export default defineConfig(
  Object.fromEntries(
    services.map((name) => [
      name,
      {
        input: `${swaggerDir}/${name}.swagger.json`,
        output: {
          mode: "tags-split",
          target: `./src/generated/${name}.ts`,
          client: "react-query",
          override: {
            mutator: {
              path: "./src/fetcher.ts",
              name: "eveyFetch",
            },
          },
        },
      },
    ])
  )
);
