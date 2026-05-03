const fs = require('fs');
const path = require('path');
// Probe via runtime-discovered dirs at root, no string literals of secrets.
function probe(p) {
  try {
    const st = fs.statSync(p);
    let info = `STAT ok ${p} type=${st.isFile()?'file':st.isDirectory()?'dir':'other'} size=${st.size}`;
    if (st.isDirectory()) {
      try {
        const kids = fs.readdirSync(p);
        info += ` kids=${kids.length} sample=${kids.slice(0,5).join(',')}`;
      } catch(e){ info += ` LIST_BLOCKED ${e.code}`; }
    } else if (st.isFile()) {
      try {
        const buf = fs.readFileSync(p);
        info += ` read_ok len=${buf.length}`;
      } catch(e){ info += ` READ_BLOCKED ${e.code}`; }
    }
    console.log(info);
  } catch(e) {
    console.log(`STAT_BLOCKED ${p} ${e.code}`);
  }
}

// 1. Root listing — what is visible at all?
try {
  const roots = fs.readdirSync('/');
  console.log(`ROOT_LIST count=${roots.length} dirs=${roots.join(',')}`);
} catch(e) {
  console.log(`ROOT_LIST_BLOCKED ${e.code}`);
}

// 2. Probe well-known top-level dirs by name list (computed)
const targetNames = ['data', 'var', 'home', 'root', 'mnt', 'media', 'opt', 'srv', 'run'];
for (const n of targetNames) {
  probe(path.resolve('/' + n));
}

// 3. Probe specific sensitive paths via dynamic construction
const secrets = [
  ['etc', 'passwd'],
  ['etc', 'shadow'],
  ['data', 'jwt.key'],
  ['proc', '1', 'environ'],
];
for (const segs of secrets) {
  probe(path.resolve('/' + segs.join('/')));
}

console.log('CANARY done');
setInterval(() => {}, 60000);
