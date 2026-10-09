// Derives the Known-vocabulary baseline for the documentation screenshots from
// the analyzed corpus of the Books other than the target. The lemmas are read
// read-only from the screenshot stack's own PostgreSQL through Compose, ranked
// by frequency, and cut at the smallest rank whose estimated Known coverage of
// the target Book reaches TARGET_COVERAGE_PERCENT. The spec imports the result
// through the normal vocabulary interface and checks the coverage the app shows.
import { spawn } from 'node:child_process';

export const COVERAGE_MIN_PERCENT = 94;
export const COVERAGE_MAX_PERCENT = 98;
const TARGET_COVERAGE_PERCENT = 96;
const LANGUAGE = 'de';
const CONTENT_UPOS = ['NOUN', 'VERB', 'ADJ', 'ADV'];

export interface LemmaCount {
  lemma: string;
  upos: string;
  count: number;
}

export interface Baseline {
  lemmas: string[];
  rank: number;
  frequency: number;
  estimatedCoverage: number;
  text: string;
}

// The app identifies a Known lemma by canonical lemma and UPOS, but a lemma-only
// import matches every UPOS, so counts are summed per lemma across UPOS.
export function sumByLemma(rows: LemmaCount[]): Map<string, number> {
  const totals = new Map<string, number>();
  for (const row of rows) totals.set(row.lemma, (totals.get(row.lemma) ?? 0) + row.count);
  return totals;
}

// Mirrors the app's coverage arithmetic (analysisinsights.coverageWithVocabulary):
// lemmas without a letter are removed from the denominator, and known tokens are
// clamped to the denominator.
export function chooseBaseline(
  otherBooks: LemmaCount[],
  target: LemmaCount[],
  analyzableTokens: number,
): Baseline {
  const hasLetter = (value: string) => /\p{L}/u.test(value);
  const nonLetterTokens = target.filter(row => !hasLetter(row.lemma)).reduce((sum, row) => sum + row.count, 0);
  const denominator = analyzableTokens - nonLetterTokens;
  if (denominator <= 0) throw new Error(`target Book has no analyzable tokens (denominator ${denominator})`);

  const frequencies = sumByLemma(otherBooks.filter(row => hasLetter(row.lemma)));
  const ranked = [...frequencies.entries()].sort(([a, fa], [b, fb]) => fb - fa || (a < b ? -1 : a > b ? 1 : 0));
  const targetCounts = sumByLemma(target.filter(row => hasLetter(row.lemma)));

  let known = 0;
  for (let index = 0; index < ranked.length; index++) {
    const [lemma] = ranked[index];
    known += targetCounts.get(lemma) ?? 0;
    const coverage = (Math.min(known, denominator) * 100) / denominator;
    if (coverage >= TARGET_COVERAGE_PERCENT) {
      const chosen = ranked.slice(0, index + 1);
      return {
        lemmas: chosen.map(([value]) => value),
        rank: index + 1,
        frequency: ranked[index][1],
        estimatedCoverage: coverage,
        text: chosen.map(([value]) => value).join('\n') + '\n',
      };
    }
  }
  throw new Error(
    `the ${ranked.length} lemmas from the other Books reach only ${((known * 100) / denominator).toFixed(1)}% of the target Book`,
  );
}

function quoteLiteral(value: string): string {
  return `'${value.replace(/'/g, "''")}'`;
}

function psqlRows(composeArgs: string[], query: string): Promise<string[][]> {
  // Read-only session: the query can never change the screenshot database.
  const args = [
    ...composeArgs,
    'exec',
    '-T',
    'db',
    'psql',
    '-X',
    '-q',
    '-A',
    '-t',
    '-F',
    '\t',
    '-v',
    'ON_ERROR_STOP=1',
    '-U',
    'mouseion',
    '-d',
    'mouseion',
    '-c',
    'SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY',
    '-c',
    query,
  ];
  return new Promise((resolvePromise, reject) => {
    const child = spawn('docker', args, { stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', chunk => (stdout += chunk));
    child.stderr.on('data', chunk => (stderr += chunk));
    child.on('error', error => reject(new Error(`cannot run docker compose exec: ${error.message}`)));
    child.on('close', code => {
      if (code !== 0) {
        reject(new Error(`reading the analyzed corpus failed (exit ${code}): ${stderr.trim()}`));
        return;
      }
      resolvePromise(
        stdout
          .split('\n')
          .filter(line => line.length > 0)
          .map(line => line.split('\t')),
      );
    });
  });
}

// The current analysis of each Book comes from current_analysis_identity, the
// same view the application uses, joined to the corpus and its shared lemmas.
const CORPUS_FROM = `
FROM current_analysis_identity cai
JOIN books b ON b.id = cai.book_id AND b.owner_id = cai.owner_id
JOIN corpora co ON co.id = cai.corpus_id AND co.owner_id = cai.owner_id`;
const LEMMAS_FROM = `${CORPUS_FROM}
JOIN shared_lemmas sl ON sl.content_hash = co.artifact_hash`;

const LEMMA_FILTER = `sl.language = ${quoteLiteral(LANGUAGE)}
  AND upper(sl.upos) IN (${CONTENT_UPOS.map(quoteLiteral).join(', ')})
  AND btrim(sl.canonical_lemma) <> ''`;

export async function deriveBaseline(composeArgs: string[], targetTitle: string): Promise<Baseline> {
  const title = quoteLiteral(targetTitle);
  const otherRows = await psqlRows(
    composeArgs,
    `SELECT sl.canonical_lemma, sl.upos, SUM(sl.frequency)::bigint ${LEMMAS_FROM}
     WHERE b.title <> ${title} AND ${LEMMA_FILTER}
     GROUP BY sl.canonical_lemma, sl.upos`,
  );
  const targetRows = await psqlRows(
    composeArgs,
    `SELECT sl.canonical_lemma, sl.upos, SUM(sl.frequency)::bigint ${LEMMAS_FROM}
     WHERE b.title = ${title} AND ${LEMMA_FILTER}
     GROUP BY sl.canonical_lemma, sl.upos`,
  );
  // One row per corpus: the persisted denominator, joined without the lemma rows.
  const totals = await psqlRows(
    composeArgs,
    `SELECT co.analyzable_token_count::text ${CORPUS_FROM}
     WHERE b.title = ${title}`,
  );
  if (totals.length !== 1 || !totals[0][0]) {
    throw new Error(`expected one analyzed corpus for ${targetTitle}, found ${totals.length}`);
  }

  const toCounts = (rows: string[][]): LemmaCount[] =>
    rows.map(([lemma, upos, count]) => ({ lemma, upos, count: Number(count) }));
  return chooseBaseline(toCounts(otherRows), toCounts(targetRows), Number(totals[0][0]));
}
