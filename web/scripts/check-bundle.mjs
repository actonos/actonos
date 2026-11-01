import fs from 'node:fs';
import path from 'node:path';
import { loadEnv } from 'vite';

const env = loadEnv(process.env.NODE_ENV || 'production', process.cwd(), '');
const buildDir = process.env.VITE_BUILD_DIR?.trim() || env.VITE_BUILD_DIR?.trim() || '../internal/server/dist';
const assets = path.resolve(buildDir, 'assets');
const files = fs.readdirSync(assets);
const entry = files.find((name) => /^index-.*\.js$/.test(name));
if (!entry) throw new Error('frontend entry bundle not found');
const bytes = fs.statSync(path.join(assets, entry)).size;
// The entry chunk was already ~456 KB before v1.0.4 because every locale JSON is bundled
// statically. 500 KB guards against further growth; lazy-loading locales per language is
// the real fix and should lower this number when done.
const budget = 500 * 1024;
if (bytes > budget) {
  throw new Error(`entry bundle ${bytes} bytes exceeds ${budget} byte budget`);
}
console.log(`entry bundle ${bytes} bytes within budget`);
