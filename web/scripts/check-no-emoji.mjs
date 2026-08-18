import fs from 'node:fs';
import path from 'node:path';

const root = path.resolve('src');
const supportedExtensions = new Set(['.ts', '.tsx', '.json', '.css', '.html']);
const emojiPattern = /\p{Extended_Pictographic}|[\u{1F1E6}-\u{1F1FF}]/gu;
const mojibakeFragments = ['\u00f0\u0178', '\u00e2\u0161', '\u00e2\u0153', '\u00ef\u00b8', '\ufffd'];
let failed = false;

function inspect(file) {
  const lines = fs.readFileSync(file, 'utf8').split(/\r?\n/);
  lines.forEach((line, index) => {
    const emoji = line.match(emojiPattern);
    const mojibake = mojibakeFragments.find((fragment) => line.includes(fragment));
    if (!emoji && !mojibake) return;
    const detail = emoji ? [...new Set(emoji)].join(' ') : mojibake;
    console.error(
      `${path.relative(process.cwd(), file).replaceAll('\\', '/')}:${index + 1}: forbidden emoji or mojibake: ${detail}`
    );
    failed = true;
  });
}

function walk(directory) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const target = path.join(directory, entry.name);
    if (entry.isDirectory()) walk(target);
    else if (supportedExtensions.has(path.extname(entry.name))) inspect(target);
  }
}

walk(root);
if (failed) process.exit(1);
console.log('emoji-free UI check ok');
