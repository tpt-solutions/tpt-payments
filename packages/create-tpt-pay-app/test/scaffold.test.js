'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync, spawnSync } = require('node:child_process');
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

test('the python template creates a runnable aiohttp project with the name filled in', () => {
  const target = path.join(tempDir(), 'pay-py');
  const files = scaffold({ name: 'pay-py', target, template: 'python-checkout' });
  assert.deepEqual(files, ['.env.example', 'README.md', 'requirements.txt', 'server.py']);
  assert.ok(fs.readFileSync(path.join(target, 'server.py'), 'utf8').includes('"pay-py"'));
  for (const f of files) {
    assert.ok(!fs.readFileSync(path.join(target, f), 'utf8').includes('{{name}}'), `${f} still has a placeholder`);
  }
  assert.ok(fs.readFileSync(path.join(target, 'requirements.txt'), 'utf8').includes('aiohttp>=3.9'));
});

test('the generated python server compiles (skipped when no python is installed)', (t) => {
  const py = ['python3', 'python'].find((cmd) => {
    const r = spawnSync(cmd, ['--version'], { encoding: 'utf8' });
    return r.status === 0;
  });
  if (!py) {
    t.skip('no python on PATH');
    return;
  }
  const target = path.join(tempDir(), 'app');
  scaffold({ name: 'app', target, template: 'python-checkout' });
  const r = spawnSync(py, ['-m', 'py_compile', path.join(target, 'server.py')], { encoding: 'utf8' });
  assert.equal(r.status, 0, r.stderr);
});

test('an unknown template is refused, and nothing is created', () => {
  const target = path.join(tempDir(), 'app');
  assert.throws(() => scaffold({ name: 'app', target, template: 'bogus' }), /unknown template "bogus"/);
  assert.throws(() => scaffold({ name: 'app', target, template: '../node-checkout' }), /unknown template/);
  assert.ok(!fs.existsSync(target), 'no directory is created for a refused template');
});

test('the default template is node-checkout', () => {
  const target = path.join(tempDir(), 'app');
  const files = scaffold({ name: 'app', target });
  assert.ok(files.includes('server.js'));
});

test('the go template creates a module named after the project', () => {
  const target = path.join(tempDir(), 'pay-go');
  const files = scaffold({ name: 'pay-go', target, template: 'go-checkout' });
  assert.deepEqual(files, ['.env.example', 'README.md', 'go.mod', 'main.go', 'verify.go', 'verify_test.go']);
  assert.equal(fs.readFileSync(path.join(target, 'go.mod'), 'utf8').split('\n')[0], 'module pay-go');
  for (const f of files) {
    assert.ok(!fs.readFileSync(path.join(target, f), 'utf8').includes('{{name}}'), `${f} still has a placeholder`);
  }
});

test('the generated go project builds and its tests pass (skipped when go is not installed)', (t) => {
  const probe = spawnSync('go', ['version'], { encoding: 'utf8' });
  if (probe.status !== 0) {
    t.skip('no go on PATH');
    return;
  }
  const target = path.join(tempDir(), 'app');
  scaffold({ name: 'app', target, template: 'go-checkout' });
  const env = { ...process.env, GOWORK: 'off', GOFLAGS: '-mod=mod' };
  const r = spawnSync('go', ['test', './...'], { cwd: target, env, encoding: 'utf8' });
  assert.equal(r.status, 0, `${r.stdout}\n${r.stderr}`);
});

test('the nextjs template creates the app with the name filled in', () => {
  const target = path.join(tempDir(), 'shop-web');
  const files = scaffold({ name: 'shop-web', target, template: 'nextjs-checkout' });
  // Paths come back in the platform's form (backslashes on Windows), so compare with path.join.
  assert.deepEqual(files, [
    '.env.example',
    'README.md',
    'app/api/checkout/route.js',
    'app/api/webhook/route.js',
    'app/layout.js',
    'app/page.js',
    'lib/checkout.js',
    'lib/tpt.js',
    'lib/webhook.js',
    'package.json',
    'test/checkout.test.js',
  ].map((p) => p.split('/').join(path.sep)));
  const pkg = JSON.parse(fs.readFileSync(path.join(target, 'package.json'), 'utf8'));
  assert.equal(pkg.name, 'shop-web');
  assert.equal(pkg.dependencies['@tpt/pay'], '^0.1.0');
  for (const f of files) {
    assert.ok(!fs.readFileSync(path.join(target, f), 'utf8').includes('{{name}}'), f + ' still has a placeholder');
  }
});
