import { defineConfig } from "orval";

const swaggerDir = "../../../gen/evey/v1";

const services = [
  "library", "user", "playback", "transcoding", "mkv",
  "metadata", "admin_media", "admin_plugin", "admin_session", "admin_system", "admin_task",
  "activity", "notification", "feed", "webhook",
  "favorite", "rating", "parental", "playback_settings", "remote_access",
  "server_config", "backup", "backup_schedule", "push", "transcode_cache", "share",
  "library_access", "collection", "music", "photo", "playlist", "syncplay",
  "plugin_prefs", "presence", "media_request",
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
          // orval 8 defaults to the fetch client, which calls the mutator as
          // (url, init); eveyFetch takes the request-config object.
          httpClient: "axios",
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
