import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const MARKER = path.join(__dirname, ".e2e-start-time");

export default function globalSetup() {
  fs.writeFileSync(MARKER, new Date().toISOString());
}
