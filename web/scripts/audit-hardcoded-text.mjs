import fs from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const root = path.resolve('src');
const findings = [];
const visibleAttributes = new Set(['alt', 'aria-label', 'placeholder', 'title']);
const ignored = new Set(['ActonOS', 'CPU', 'RAM', 'UTF-8']);
const ignoredPatterns = [
  /^v?\d+\.\d+\.\d+(?:[- ().A-Za-z]+)?$/,
  /^[A-Z][A-Z0-9_]+=\S+$/,
  /^agent_[a-z0-9_]+$/,
];

function hasWords(value) {
  const text = value.replace(/\s+/g, ' ').trim();
  return (
    text.length > 1 &&
    /[A-Za-zÀ-ỹ]/u.test(text) &&
    !ignored.has(text) &&
    !ignoredPatterns.some((pattern) => pattern.test(text))
  );
}

function visitFile(file) {
  const sourceText = fs.readFileSync(file, 'utf8');
  const source = ts.createSourceFile(file, sourceText, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
  function report(node, text) {
    const pos = source.getLineAndCharacterOfPosition(node.getStart(source));
    findings.push({
      file: path.relative(process.cwd(), file).replaceAll('\\', '/'),
      line: pos.line + 1,
      text: text.replace(/\s+/g, ' ').trim(),
    });
  }
  function visit(node) {
    if (ts.isJsxText(node) && hasWords(node.text)) {
      report(node, node.text);
    } else if (
      ts.isJsxAttribute(node) &&
      visibleAttributes.has(node.name.text) &&
      node.initializer &&
      ts.isStringLiteral(node.initializer) &&
      hasWords(node.initializer.text)
    ) {
      report(node, `${node.name.text}="${node.initializer.text}"`);
    } else if (
      ts.isJsxExpression(node) &&
      node.expression &&
      ts.isStringLiteral(node.expression) &&
      hasWords(node.expression.text)
    ) {
      report(node, node.expression.text);
    }
    ts.forEachChild(node, visit);
  }
  visit(source);
}

function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const target = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(target);
    else if (/\.tsx$/.test(entry.name) && !/\.test\.tsx$/.test(entry.name)) visitFile(target);
  }
}

walk(root);
for (const item of findings) {
  console.log(`${item.file}:${item.line}: ${item.text}`);
}
console.log(`hardcoded visible strings: ${findings.length}`);
if (process.argv.includes('--fail') && findings.length > 0) process.exit(1);
