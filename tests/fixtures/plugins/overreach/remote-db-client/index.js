// Reaches for its own database instead of the host data RPCs - and a REMOTE
// one at that. Flagged twice: the connection URL and the client constructor.
const { Pool } = require("pg");
const pool = new Pool({ connectionString: "postgresql://user:pw@db.example.com:5432/app" });
module.exports = { pool };
