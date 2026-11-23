'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { scaffold, validateName } = require('../lib/scaffold');

function tempDir() {
  return fs.mkdtempSync(path.join(os.tmpdir(), 'create-tpt-pay-app-'));
}

test('creates the project with the name filled in and no template suffixes left', () => {
  const root = tempDir();
  const target = path.join(root, 'shop-api');
  const files = scaffold({ name: 'shop-api', target });

  assert.deepEqual(files, [
    '.env.example',
    'README.md',
    'package.json',
    'server.js',
  ]);
  const pkg = JSON.parse(fs.readFileSync(path.join(target, 'package.json'), 'utf8'));
  assert.equal(pkg.name, 'shop-api');
  assert.equal(pkg.dependencies['@tpt/pay'], '^0.1.0');

  for (const f of files) {
    const text = fs.readFileSync(path.join(target, f), 'utf8');
    assert.ok(!text.includes('{{name}}'), `${f} still has a placeholder`);
  }
  assert.ok(fs.readFileSync(path.join(target, 'README.md'), 'utf8').startsWith('# shop-api'));
});

test('the generated server is valid JavaScript', () => {
  const target = path.join(tempDir(), 'app');
  scaffold({ name: 'app', target });
  // node --check parses the file without running it.
  execFileSync(process.execPath, ['--check', path.join(target, 'server.js')]);
});

test('refuses a target that is not empty, and writes nothing into it', () => {
  const target = tempDir();
  fs.writeFileSync(path.join(target, 'keep.txt'), 'mine');
  assert.throws(() => scaffold({ name: 'app', target }), /is not empty/);
  assert.deepEqual(fs.readdirSync(target), ['keep.txt']);
});

test('refuses a target that is a file', () => {
  const dir = tempDir();
  const file = path.join(dir, 'occupied');
  fs.writeFileSync(file, 'x');
  assert.throws(() => scaffold({ name: 'app', target: file }), /not a directory/);
});

test('an empty existing directory is fine', () => {
  const target = tempDir();
  assert.doesNotThrow(() => scaffold({ name: 'app', target }));
});

test('project names must be simple lowercase names', () => {
  for (const good of ['app', 'shop-api', 'a1', '0day']) {
    assert.equal(validateName(good), good);
  }
  for (const bad of ['', 'Shop', '-app', 'app name', 'app/../x', 'a'.repeat(65), undefined]) {
    assert.throws(() => validateName(bad), /project name/, String(bad));
  }
});
