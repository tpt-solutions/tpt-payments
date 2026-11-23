// Creates a project from a template. No dependencies beyond Node's standard library.
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const TEMPLATE_DIR = path.join(__dirname, '..', 'templates', 'node-checkout');
const NAME_PATTERN = /^[a-z0-9][a-z0-9-]{0,63}$/;

/** Checks a project name: lowercase letters, digits and hyphens, starting with a letter or digit. */
function validateName(name) {
  if (typeof name !== 'string' || !NAME_PATTERN.test(name)) {
    throw new Error(
      'project name must be 1-64 lowercase letters, digits or hyphens, and start with a letter or digit'
    );
  }
  return name;
}

/**
 * Copies the template into `target`. Refuses a target that exists and is not empty, so nothing is
 * overwritten. Files ending in .tmpl have {{name}} replaced and the suffix removed. Returns the
 * list of files written, relative to `target`.
 */
function scaffold({ name, target }) {
  validateName(name);
  if (fs.existsSync(target)) {
    if (!fs.statSync(target).isDirectory()) {
      throw new Error(`${target} exists and is not a directory`);
    }
    if (fs.readdirSync(target).length > 0) {
      throw new Error(`${target} is not empty; choose another name or an empty directory`);
    }
  } else {
    fs.mkdirSync(target, { recursive: true });
  }

  const written = [];
  copyTree(TEMPLATE_DIR, target, '', { name }, written);
  return written.sort();
}

function copyTree(srcDir, destDir, rel, vars, written) {
  for (const entry of fs.readdirSync(path.join(srcDir, rel), { withFileTypes: true })) {
    const childRel = path.join(rel, entry.name);
    if (entry.isDirectory()) {
      fs.mkdirSync(path.join(destDir, childRel), { recursive: true });
      copyTree(srcDir, destDir, childRel, vars, written);
      continue;
    }
    let outRel = childRel;
    let content = fs.readFileSync(path.join(srcDir, childRel), 'utf8');
    if (outRel.endsWith('.tmpl')) {
      outRel = outRel.slice(0, -'.tmpl'.length);
      content = content.split('{{name}}').join(vars.name);
    }
    fs.writeFileSync(path.join(destDir, outRel), content);
    written.push(outRel);
  }
}

module.exports = { scaffold, validateName, TEMPLATE_DIR };
