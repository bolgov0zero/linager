// Assembles site/content (the VitePress source) from the documentation of the repository.
// docs/en -> /, docs/ru -> /ru/, docs/architecture.md (Russian) -> /ru/architecture,
// site/static (landing pages, introduction) and the screenshots -> public assets.
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const site = path.resolve(here, '..');
const repo = path.resolve(site, '..');
const out = path.join(site, 'content');

const REPO_URL = 'https://github.com/bolgov0zero/linager';
// docs pages that become site pages (README.md files are indexes of the folders and are replaced by the sidebar)
const PAGES = ['install', 'proxy', 'access', 'connection', 'app', 'modules', 'menu', 'console', 'keys', 'windows', 'updates', 'security', 'architecture'];

fs.rmSync(out, { recursive: true, force: true });
fs.mkdirSync(path.join(out, 'ru'), { recursive: true });
fs.mkdirSync(path.join(out, 'public'), { recursive: true });

/** Links that work on GitHub and must work on the site too. */
function fixLinks(md) {
  return md
    .replaceAll('](../../SECURITY.md)', `](${REPO_URL}/blob/main/SECURITY.md)`)
    .replaceAll('](../screenshots/', '](/screenshots/')
    .replaceAll('](../design/', `](${REPO_URL}/tree/main/docs/design/`)
    .replace(/\]\(\.\.\/(?:en|ru)\/\)/g, '](/)');
}

function copyPage(src, dst) {
  if (!fs.existsSync(src)) return false;
  fs.writeFileSync(dst, fixLinks(fs.readFileSync(src, 'utf8')));
  return true;
}

for (const p of PAGES) {
  copyPage(path.join(repo, 'docs/en', `${p}.md`), path.join(out, `${p}.md`));
  const ru = path.join(repo, 'docs/ru', `${p}.md`);
  // the Russian architecture document lives one level up (it is the original text)
  if (!copyPage(ru, path.join(out, 'ru', `${p}.md`)) && p === 'architecture') {
    copyPage(path.join(repo, 'docs/architecture.md'), path.join(out, 'ru', 'architecture.md'));
  }
}

// landing pages and the introduction
for (const f of fs.readdirSync(path.join(site, 'static'))) {
  const from = path.join(site, 'static', f);
  if (fs.statSync(from).isDirectory()) {
    for (const g of fs.readdirSync(from)) fs.copyFileSync(path.join(from, g), path.join(out, f, g));
  } else if (f.endsWith('.md')) {
    fs.copyFileSync(from, path.join(out, f));
  } else {
    fs.copyFileSync(from, path.join(out, 'public', f));
  }
}

// public assets: screenshots, fonts (with their licence), the icons of the web app
fs.cpSync(path.join(repo, 'docs/screenshots'), path.join(out, 'public/screenshots'), { recursive: true });
fs.mkdirSync(path.join(out, 'public/fonts'), { recursive: true });
for (const f of ['MedeoMonoVariable.woff2', 'MedeoPixelVariable.woff', 'LICENSE-Medeo-OFL.txt']) {
  fs.copyFileSync(path.join(repo, 'docs/design/mockup/fonts', f), path.join(out, 'public/fonts', f));
}
console.log('site content ready:', out);
