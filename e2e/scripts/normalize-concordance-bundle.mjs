import { readFile, writeFile } from 'node:fs/promises';

const bundlePath = '../internal/webapp/static/concordance.js';
const source = await readFile(bundlePath, 'utf8');
const replacements = [
  ['[ \t\n\\f\\r', String.raw`[\x20\t\n\f\r`],
  ['[^ \t\n\\f\\r', String.raw`[^\x20\t\n\f\r`],
];

for (const [literalWhitespace, escapedWhitespace] of replacements) {
  if (!source.includes(literalWhitespace)) {
    throw new Error('Lit bundle no longer contains the expected whitespace expression');
  }
}

await writeFile(bundlePath, replacements.reduce((bundle, [literal, escaped]) => bundle.replace(literal, escaped), source));
