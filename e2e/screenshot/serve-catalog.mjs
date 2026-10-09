// Static file server for the screenshot catalog. It runs inside the
// node:22-alpine container from compose.screenshot.yaml with no dependencies.
// Extensionless URLs resolve to a sibling `.atom` file, so `/opds` serves
// `opds.atom` with the Atom media type while the feed files keep readable names.
import { createServer } from 'node:http';
import { stat, readFile } from 'node:fs/promises';
import { join, resolve, sep } from 'node:path';

const root = resolve(process.env.CATALOG_ROOT ?? '/srv/catalog');
const port = Number(process.env.PORT ?? 8080);

const MEDIA_TYPES = {
  '.atom': 'application/atom+xml;profile=opds-catalog',
  '.epub': 'application/epub+zip',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
};

async function isFile(path) {
  try {
    return (await stat(path)).isFile();
  } catch {
    return false;
  }
}

async function resolveFile(urlPath) {
  let decoded;
  try {
    decoded = decodeURIComponent(urlPath.split('?')[0]);
  } catch {
    return null;
  }
  const relative = decoded.replace(/^\/+/, '');
  const base = join(root, relative);
  for (const candidate of [base, `${base}.atom`]) {
    if (!candidate.startsWith(root + sep)) return null;
    if (await isFile(candidate)) return candidate;
  }
  return null;
}

const server = createServer(async (request, response) => {
  if (request.method !== 'GET' && request.method !== 'HEAD') {
    response.writeHead(405, { Allow: 'GET, HEAD' }).end();
    return;
  }
  const file = await resolveFile(request.url ?? '/');
  if (!file) {
    response.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' }).end('not found\n');
    return;
  }
  const body = await readFile(file);
  const extension = file.slice(file.lastIndexOf('.'));
  response.writeHead(200, {
    'Content-Type': MEDIA_TYPES[extension] ?? 'application/octet-stream',
    'Content-Length': body.length,
  });
  response.end(request.method === 'HEAD' ? undefined : body);
});

server.listen(port, '0.0.0.0', () => {
  console.log(`screenshot catalog serving ${root} on :${port}`);
});
