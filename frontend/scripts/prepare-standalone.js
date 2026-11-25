// Copies what the standalone server needs next to it. `next build` with `output: 'standalone'`
// writes .next/standalone/server.js, but leaves the static assets and public/ where they were.
// Without this copy the pages load without their JavaScript and CSS.
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const root = path.join(__dirname, '..');
const standalone = path.join(root, '.next', 'standalone');

if (!fs.existsSync(path.join(standalone, 'server.js'))) {
  console.error('no standalone build found: run `npm run build` first');
  process.exit(1);
}

function copyDir(from, to) {
  fs.rmSync(to, { recursive: true, force: true });
  fs.mkdirSync(path.dirname(to), { recursive: true });
  fs.cpSync(from, to, { recursive: true });
}

copyDir(path.join(root, '.next', 'static'), path.join(standalone, '.next', 'static'));
if (fs.existsSync(path.join(root, 'public'))) {
  copyDir(path.join(root, 'public'), path.join(standalone, 'public'));
}
console.log('standalone build ready: .next/standalone/server.js');
