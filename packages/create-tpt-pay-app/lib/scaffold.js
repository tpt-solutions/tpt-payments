// Creates a project from a template. No dependencies beyond Node's standard library.
'use strict';

const fs = require('node:fs');
const path = require('node:path');

const TEMPLATES_ROOT = path.join(__dirname, '..', 'templates');
const NAME_PATTERN = /^[a-z0-9][a-z0-9-]{0,63}$/;

/**
 * The templates a project can be made from. The name is looked up here, never joined into a path,
 * so a template argument cannot point the copy at another directory.
 */
const TEMPLATES = {
  'node-checkout': 'Node.js (plain http and @tpt/pay): checkout and webhook receiver',
  'python-checkout': 'Python (aiohttp and the tpt-payments client): checkout and webhook receiver',
  'go-checkout': 'Go (standard library): checkout and signed webhook receiver',
  'nextjs-checkout': 'Next.js (App Router): checkout form, checkout route and signed webhook route',
};
const DEFAULT_TEMPLATE = 'node-checkout';
const TEMPLATE_DIR = path.join(TEMPLATES_ROOT, DEFAULT_TEMPLATE);

/** Checks a project name: lowercase letters, digits and hyphens, starting with a letter or digit. */
function validateName(name) {
  if (typeof name !== 'string' || !NAME_PATTERN.test(name)) {
    throw new Error(
      'project name must be 1-64 lowercase letters, digits or hyphens, and start with a letter or digit'
    );
  }
  return name;
}

/** Returns the template directory for a template name, or throws for an unknown one. */
function templateDir(template) {
  if (!Object.prototype.hasOwnProperty.call(TEMPLATES, template)) {
    throw new Error(`unknown template "${template}"; choose one of: ${Object.keys(TEMPLATES).join(', ')}`);
  }
  return path.join(TEMPLATES_ROOT, template);
}

/**
 * Copies a template into `target`. Refuses a target that exists and is not empty, so nothing is
 * overwritten. Files ending in .tmpl have {{name}} replaced and the suffix removed. Returns the
 * list of files written, relative to `target`.
 */
function scaffold({ name, target, template = DEFAULT_TEMPLATE }) {
  validateName(name);
  const src = templateDir(template);
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
  copyTree(src, target, '', { name }, written);
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

module.exports = { scaffold, validateName, templateDir, TEMPLATES, DEFAULT_TEMPLATE, TEMPLATE_DIR };
