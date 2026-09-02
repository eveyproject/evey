// A well-meaning author who shipped a whole stack as a "plugin": it tries to
// bring up its own containers. None of this can work in the sandbox, and the
// admin must be told before install, not left to debug a silent failure.
const { execSync } = require("child_process");
execSync("docker compose -f ./stack/docker-compose.yml up -d");
