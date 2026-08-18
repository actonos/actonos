import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve('src/locales');
const enFiles = fs.readdirSync(path.join(root, 'en')).filter((name) => name.endsWith('.json'));
let failed = false;

function flatten(value, prefix = '') {
  return Object.entries(value).flatMap(([key, child]) => {
    const next = prefix ? `${prefix}.${key}` : key;
    return child && typeof child === 'object' && !Array.isArray(child)
      ? flatten(child, next)
      : [next];
  });
}

for (const file of enFiles) {
  const viPath = path.join(root, 'vi', file);
  if (!fs.existsSync(viPath)) {
    console.error(`missing Vietnamese namespace: ${file}`);
    failed = true;
    continue;
  }
  const en = flatten(JSON.parse(fs.readFileSync(path.join(root, 'en', file), 'utf8')));
  const vi = flatten(JSON.parse(fs.readFileSync(viPath, 'utf8')));
  for (const key of en.filter((key) => !vi.includes(key))) {
    console.error(`${file}: missing vi key ${key}`);
    failed = true;
  }
  for (const key of vi.filter((key) => !en.includes(key))) {
    console.error(`${file}: missing en key ${key}`);
    failed = true;
  }
}

if (failed) process.exit(1);
console.log('locale parity ok');
