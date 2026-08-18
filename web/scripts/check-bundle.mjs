import fs from 'node:fs';
import path from 'node:path';

const assets = path.resolve('../internal/server/dist/assets');
const files = fs.readdirSync(assets);
const entry = files.find((name) => /^index-.*\.js$/.test(name));
if (!entry) throw new Error('frontend entry bundle not found');
const bytes = fs.statSync(path.join(assets, entry)).size;
const budget = 350 * 1024;
if (bytes > budget) {
  throw new Error(`entry bundle ${bytes} bytes exceeds ${budget} byte budget`);
}
console.log(`entry bundle ${bytes} bytes within budget`);
