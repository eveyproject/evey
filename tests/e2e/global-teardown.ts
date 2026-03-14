import { execSync } from "child_process";
import * as fs from "fs";
import * as path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const MARKER = path.join(__dirname, ".e2e-start-time");
const STORAGE_STATE = path.join(__dirname, ".auth-state.json");

const PG_CONTAINER = process.env.PG_CONTAINER ?? "evey-postgres";
const PG_USER = process.env.PG_USER ?? "evey";
const PG_DB = process.env.PG_DB ?? "evey";

function sql(query: string) {
  execSync(
    `docker exec ${PG_CONTAINER} psql -U ${PG_USER} -d ${PG_DB} -c "${query}"`,
    { stdio: "pipe" },
  );
}

export default function globalTeardown() {
  let since: string;
  try {
    since = fs.readFileSync(MARKER, "utf-8").trim();
    fs.unlinkSync(MARKER);
  } catch {
    return;
  }
  try { fs.unlinkSync(STORAGE_STATE); } catch { /* already gone */ }
  try {
    sql(`DELETE FROM audit_log WHERE created_at >= '${since}'`);
    sql(`DELETE FROM play_sessions WHERE started_at >= '${since}'`);
    sql(`DELETE FROM login_history WHERE created_at >= '${since}'`);
    sql(`DELETE FROM collections WHERE created_at >= '${since}'`);
    sql(`DELETE FROM playlists WHERE created_at >= '${since}'`);
    sql(`DELETE FROM profiles WHERE user_id IN (SELECT id FROM users WHERE username LIKE 'e2e_%')`);
    sql(`DELETE FROM users WHERE username LIKE 'e2e_%'`);
    console.log("E2E teardown: cleaned test data since " + since);
  } catch (err) {
    console.warn("E2E teardown failed (non-fatal):", (err as Error).message);
  }
}
