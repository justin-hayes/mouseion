#!/usr/bin/env node
// Documentation screenshot of My Books, run by `make screenshot-my-books`.
//
// 1. Fetches and verifies the pinned Project Gutenberg books (cached under .tmp).
// 2. Prepares EPUB 3 files, typographic covers, and a static OPDS catalog.
// 3. Starts the existing Compose definition under its own project name with a
//    localhost-only port, generated secrets, German only, and the LLM disabled.
// 4. Runs the learner workflow in Playwright (e2e/screenshot/my-books.spec.ts),
//    which analyzes the scenario Books and reads one to completion.
// 5. Writes the optimized screenshots to doc/images/my-books.png and
//    doc/images/reading.png and stops the stack, keeping the Stanza model volume.
//
// `--clean` removes the project's containers and volumes, then exits.
import { spawn } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { chmod, mkdir, readFile, rename, rm, stat, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { CATALOG_ROOT_PATH } from './opds.mjs';
import { optimizePng } from './png.mjs';
import { ScreenshotError, prepareLibrary } from './prepare.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const e2eDir = resolve(here, '..');
const repo = resolve(e2eDir, '..');
const PROJECT = 'mouseion-screenshot';
const STANZA_VOLUME = 'mouseion-screenshot-stanza-data';
const HUGGINGFACE_VOLUME = 'mouseion-screenshot-huggingface-data';
const DEFAULT_PORT = 18080;
const USERNAME = 'screenshot-learner';
const CATALOG_NAME = 'Scenario catalog';
const CATALOG_URL = `http://catalog:8080${CATALOG_ROOT_PATH}`;
const STACK_READY_TIMEOUT_SECONDS = 1800;

const scratch = join(repo, '.tmp', 'screenshot');
const paths = {
  sources: join(scratch, 'sources'),
  books: join(scratch, 'books'),
  covers: join(scratch, 'covers'),
  catalog: join(scratch, 'catalog'),
  results: join(scratch, 'test-results'),
  out: join(scratch, 'out'),
  run: join(scratch, 'run'),
};
const envFile = join(paths.run, 'compose.env');
const manifestPath = join(here, 'manifest.json');
const myBooksOutput = join(repo, 'doc', 'images', 'my-books.png');
const readingOutput = join(repo, 'doc', 'images', 'reading.png');
const composeFiles = ['-f', join(repo, 'compose.yaml'), '-f', join(here, 'compose.screenshot.yaml')];
const log = message => console.log(message);

// Child processes never inherit MOUSEION_* or COMPOSE_* settings from the
// maintainer's shell, so only the run env file configures the isolated stack.
function childEnvironment(extra = {}) {
  const env = {};
  for (const [key, value] of Object.entries(process.env)) {
    if (/^(MOUSEION_|COMPOSE_)/.test(key)) continue;
    env[key] = value;
  }
  return { ...env, ...extra };
}

function run(command, args, { cwd = repo, env, capture = false } = {}) {
  return new Promise((resolvePromise, reject) => {
    const child = spawn(command, args, {
      cwd,
      env: childEnvironment(env),
      stdio: capture ? ['ignore', 'pipe', 'pipe'] : 'inherit',
    });
    let stdout = '';
    let stderr = '';
    if (capture) {
      child.stdout.on('data', chunk => (stdout += chunk));
      child.stderr.on('data', chunk => (stderr += chunk));
    }
    child.on('error', error => reject(new ScreenshotError(`cannot run ${command}: ${error.message}`)));
    child.on('close', code => resolvePromise({ code, stdout: stdout.trim(), stderr: stderr.trim() }));
  });
}

function compose(args, options = {}) {
  return run(
    'docker',
    ['compose', '-p', PROJECT, '--project-directory', repo, '--env-file', envFile, ...composeFiles, ...args],
    options,
  );
}

async function writeRunEnvironment(port) {
  await mkdir(paths.run, { recursive: true });
  const password = randomBytes(18).toString('base64url');
  const lines = [
    `MOUSEION_DB_PASSWORD=${randomBytes(24).toString('hex')}`,
    `MOUSEION_SECRET=${randomBytes(48).toString('base64url')}`,
    `MOUSEION_SCREENSHOT_PORT=${port}`,
    `MOUSEION_SCREENSHOT_CATALOG_DIR=${paths.catalog}`,
    'MOUSEION_COOKIE_SECURE=false',
    '',
  ];
  await writeFile(envFile, lines.join('\n'), { mode: 0o600 });
  await chmod(envFile, 0o600);
  return { password };
}

function parseComposeVersion(text) {
  const match = /(\d+)\.(\d+)\.(\d+)/.exec(text);
  return match ? match.slice(1).map(Number) : null;
}

async function preflight({ needsDownload }) {
  const major = Number(process.versions.node.split('.')[0]);
  if (major < 20) {
    throw new ScreenshotError(`Node.js 20 or newer is required; found ${process.versions.node}. Install Node.js 20 and rerun.`);
  }

  const docker = await run('docker', ['version', '--format', '{{.Server.Version}}'], { capture: true });
  if (docker.code !== 0) {
    throw new ScreenshotError('Docker is unavailable. Start the Docker daemon and make sure this user can run docker, then rerun.');
  }

  const composeVersion = await run('docker', ['compose', 'version', '--short'], { capture: true });
  const version = parseComposeVersion(composeVersion.stdout);
  if (composeVersion.code !== 0 || !version) {
    throw new ScreenshotError('Docker Compose v2 is unavailable. Install the Docker Compose plugin (v2.24.4 or newer) and rerun.');
  }
  const [compMajor, compMinor, compPatch] = version;
  const supportsOverride = compMajor > 2 || (compMajor === 2 && (compMinor > 24 || (compMinor === 24 && compPatch >= 4)));
  if (!supportsOverride) {
    throw new ScreenshotError(`Docker Compose ${version.join('.')} is too old; the override needs v2.24.4 or newer (!override).`);
  }

  if (needsDownload) {
    try {
      const response = await fetch('https://www.gutenberg.org/', {
        method: 'HEAD',
        signal: AbortSignal.timeout(15_000),
        headers: { 'user-agent': 'MouseionDocsScreenshot/1.0' },
      });
      if (response.status >= 500) throw new Error(`HTTP ${response.status}`);
    } catch (error) {
      throw new ScreenshotError(
        `Network access to www.gutenberg.org is unavailable (${error.message}). Connect to the network, or place the pinned source files in ${paths.sources} and rerun.`,
      );
    }
  }
}

async function sourcesNeedDownload(manifest) {
  for (const book of manifest.books) {
    try {
      await stat(join(paths.sources, `${book.gutenberg}.txt`));
    } catch {
      return true;
    }
  }
  return false;
}

async function cleanStack() {
  await preflight({ needsDownload: false });
  await writeRunEnvironment(DEFAULT_PORT);
  try {
    log(`Removing containers and volumes for compose project ${PROJECT}`);
    const down = await compose(['down', '--volumes', '--remove-orphans']);
    if (down.code !== 0) throw new ScreenshotError('docker compose down failed; see the output above.');
    for (const volume of [STANZA_VOLUME, HUGGINGFACE_VOLUME]) {
      const removed = await run('docker', ['volume', 'rm', volume], { capture: true });
      log(removed.code === 0 ? `Removed model volume ${volume}` : `Volume ${volume} was already absent`);
    }
  } finally {
    await rm(envFile, { force: true });
  }
}

async function waitForLogin(baseURL, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(new URL('/login', baseURL), { signal: AbortSignal.timeout(5_000) });
      if (response.ok) return;
    } catch {
      // The web container is still starting; keep polling until the deadline.
    }
    await new Promise(resolvePromise => setTimeout(resolvePromise, 2_000));
  }
  throw new ScreenshotError(`The web server at ${baseURL} did not respond within ${timeoutMs / 1000}s.`);
}

async function runWorkflow({ baseURL, password }) {
  const env = {
    MOUSEION_SCREENSHOT_BASE_URL: baseURL,
    MOUSEION_SCREENSHOT_USERNAME: USERNAME,
    MOUSEION_SCREENSHOT_PASSWORD: password,
    MOUSEION_SCREENSHOT_CATALOG_NAME: CATALOG_NAME,
    MOUSEION_SCREENSHOT_CATALOG_URL: CATALOG_URL,
    MOUSEION_SCREENSHOT_MANIFEST: manifestPath,
    MOUSEION_SCREENSHOT_OUTPUT: join(paths.out, 'my-books.raw.png'),
    MOUSEION_SCREENSHOT_READING_OUTPUT: join(paths.out, 'reading.raw.png'),
    MOUSEION_SCREENSHOT_RESULTS: paths.results,
  };
  const result = await run('npx', ['playwright', 'test', '-c', 'screenshot/playwright.config.ts'], { cwd: e2eDir, env });
  if (result.code !== 0) {
    throw new ScreenshotError(`The My Books assertions failed (Playwright exit ${result.code}). No screenshot was written.`);
  }
}

async function writeScreenshot(rawName, output) {
  const raw = await readFile(join(paths.out, rawName));
  const optimized = optimizePng(raw);
  await mkdir(dirname(output), { recursive: true });
  const temporary = `${output}.partial`;
  await writeFile(temporary, optimized);
  await rename(temporary, output);
  log(`Wrote ${output} (${raw.length} -> ${optimized.length} bytes)`);
}

async function createModelVolumes() {
  for (const volume of [STANZA_VOLUME, HUGGINGFACE_VOLUME]) {
    const created = await run('docker', ['volume', 'create', volume], { capture: true });
    if (created.code !== 0) throw new ScreenshotError(`Cannot create the model volume ${volume}: ${created.stderr}`);
  }
}

async function screenshot() {
  const manifest = JSON.parse(await readFile(manifestPath, 'utf8'));
  await preflight({ needsDownload: await sourcesNeedDownload(manifest) });
  const port = Number(process.env.MOUSEION_SCREENSHOT_PORT ?? DEFAULT_PORT);
  const { password } = await writeRunEnvironment(port);
  await mkdir(paths.out, { recursive: true });

  try {
    log('Preparing the pinned public-domain books');
    await prepareLibrary({ manifest, paths, log });

    log(`Starting the isolated stack (compose project ${PROJECT}, 127.0.0.1:${port})`);
    await createModelVolumes();
    // A fresh database gives every run a new learner; the model volumes are
    // external, so `down --volumes` leaves them in place.
    await compose(['down', '--volumes', '--remove-orphans']);
    const up = await compose(['up', '-d', '--build', '--wait', '--wait-timeout', String(STACK_READY_TIMEOUT_SECONDS)]);
    if (up.code !== 0) {
      const logs = await compose(['logs', '--no-color', '--tail', '120', 'web', 'nlp', 'catalog'], { capture: true });
      console.error(`--- recent service logs ---\n${logs.stdout}\n${logs.stderr}`);
      throw new ScreenshotError('The isolated stack did not become ready; see the Compose output above. No screenshot was written.');
    }

    const baseURL = `http://127.0.0.1:${port}`;
    await waitForLogin(baseURL, 180_000);
    log('Driving My Books in Playwright');
    await runWorkflow({ baseURL, password });
    await writeScreenshot('my-books.raw.png', myBooksOutput);
    await writeScreenshot('reading.raw.png', readingOutput);
  } finally {
    // Stop containers and networks only; the model volumes and the database are
    // kept until the next run recreates the database or `--clean` is used.
    const stopped = await compose(['down']);
    if (stopped.code === 0) log('Stopped the isolated stack; the Stanza model volume was kept.');
    await rm(envFile, { force: true });
  }
}

async function main() {
  if (process.argv.includes('--clean')) {
    await cleanStack();
    log('Clean-up complete.');
    return;
  }
  await screenshot();
}

main().catch(error => {
  console.error(`screenshot: ${error instanceof ScreenshotError ? error.message : (error.stack ?? error)}`);
  process.exitCode = 1;
});
