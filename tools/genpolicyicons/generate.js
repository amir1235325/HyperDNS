// tools/genpolicyicons/generate.js — generator for the built-in policy icons
// (v2.7). Brand policies get their REAL service logo as a single-path SVG from
// the Simple Icons collection (CC0; the logos remain their owners' trademarks —
// used here the way any panel references the services it routes). Category
// policies (ai, social, adblock, …) have no brand, so they keep a theme-adaptive
// Feather glyph extracted from the vendored feather.min.js — the same geometry
// the dashboard always showed.
//
// Every output is standalone, 24×24, and coloured with currentColor so it
// follows whatever theme the panel renders. Run from the repo root:
//   node tools/genpolicyicons/generate.js
// Brand fetches hit cdn.jsdelivr.net once and are cached in this directory
// (tools/genpolicyicons/.cache) so re-runs are offline-safe.
'use strict';
const fs = require('fs');
const path = require('path');
const feather = require(path.join(__dirname, '..', '..', 'web', 'js', 'feather.min.js'));

// id -> { brand: 'simple-icons slug' } or { feather: 'feather icon name' }.
const MAP = {
  riot: { brand: 'riotgames' },
  epic: { brand: 'epicgames' },
  steam: { brand: 'steam' },
  pubg: { brand: 'pubg' },
  ea: { brand: 'ea' },
  blizzard: { brand: 'battledotnet' }, // Battle.net — Blizzard's own brand mark
  ubisoft: { brand: 'ubisoft' },
  rockstar: { brand: 'rockstargames' },
  playstation: { brand: 'playstation' },
  roblox: { brand: 'roblox' },
  discord: { brand: 'discord' },
  spotify: { brand: 'spotify' },
  twitch: { brand: 'twitch' },
  kick: { brand: 'kick' },
  google: { brand: 'google' },
  soundcloud: { brand: 'soundcloud' },
  // xbox: simple-icons dropped Microsoft marks at their request (v13) — the
  // Feather grid glyph stays. call_of_duty and supercell were never in the
  // collection; they keep their card glyphs like the other categories.
  call_of_duty: { feather: 'flag' },
  supercell: { feather: 'star' },
  xbox: { feather: 'grid' },
  ai: { feather: 'cpu' },
  social: { feather: 'share-2' },
  anime_gacha: { feather: 'feather' },
  coop_survival: { feather: 'users' },
  platforms_extra: { feather: 'layers' },
  shooters_extra: { feather: 'crosshair' },
  sports_racing: { feather: 'dribbble' },
  dev403: { feather: 'code' },
  adblock: { feather: 'slash' },
  familysafe: { feather: 'eye-off' },
  downloads: { feather: 'download-cloud' },
};

const cacheDir = path.join(__dirname, '.cache');
const outDir = path.join(__dirname, '..', '..', 'presets', 'icons');
fs.mkdirSync(outDir, { recursive: true });
fs.mkdirSync(cacheDir, { recursive: true });

async function fetchBrand(slug) {
  const cache = path.join(cacheDir, `${slug}.svg`);
  if (fs.existsSync(cache)) return fs.readFileSync(cache, 'utf8');
  const url = `https://cdn.jsdelivr.net/npm/simple-icons@13/icons/${slug}.svg`;
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${slug}: HTTP ${res.status}`);
  const body = await res.text();
  fs.writeFileSync(cache, body);
  return body;
}

// simple-icons documents the path with no fill attribute (SVG default = black),
// so the fill is set explicitly to currentColor; the <title> is dropped because
// the <img> in the dashboard carries its own alt text.
function normalizeBrand(svg, id) {
  const inner = svg.slice(svg.indexOf('>') + 1, svg.lastIndexOf('</svg>')).replace(/<title>[\s\S]*?<\/title>/, '');
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" role="img" aria-label="${id}">` + inner + `</svg>\n`;
}

// Feather's toSvg carries fixed width/height and a class; the standalone file
// keeps only the geometry and the theme-following stroke.
function normalizeFeather(svg, name) {
  const inner = svg.slice(svg.indexOf('>') + 1, svg.lastIndexOf('</svg>'));
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">` + inner + `</svg>\n`;
}

(async () => {
  const seen = new Map();
  let brands = 0, glyphs = 0;
  for (const [id, src] of Object.entries(MAP)) {
    let doc;
    if (src.brand) {
      doc = normalizeBrand(await fetchBrand(src.brand), id);
      brands++;
    } else {
      const icon = feather.icons[src.feather];
      if (!icon) throw new Error(`feather has no icon named ${src.feather}`);
      doc = normalizeFeather(icon.toSvg(), src.feather);
      glyphs++;
    }
    const prev = seen.get(src.brand || src.feather);
    if (prev) throw new Error(`duplicate glyph ${src.brand || src.feather}: ${prev} and ${id}`);
    seen.set(src.brand || src.feather, id);
    fs.writeFileSync(path.join(outDir, `${id}.svg`), doc);
  }
  console.log(`wrote ${brands + glyphs} icons (${brands} brand logos, ${glyphs} feather glyphs) to ${outDir}`);
})();
