import { spawn, type ChildProcess } from 'node:child_process';
import net from 'node:net';
import { setTimeout as delay } from 'node:timers/promises';
import { test as base, expect } from '@playwright/test';

async function availableLoopbackPort(): Promise<number> {
  const server = net.createServer();
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });
  const address = server.address();
  await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
  if (!address || typeof address === 'string') throw new Error('Could not select a loopback port for fixture server');
  return address.port;
}

async function waitForFixture(url: string, server: ChildProcess): Promise<void> {
  const deadline = Date.now() + 30_000;
  let lastError: unknown;
  server.once('error', error => { lastError = error; });
  while (Date.now() < deadline) {
    if (lastError && server.pid === undefined) throw new Error(`Could not start fixture server: ${String(lastError)}`);
    if (server.exitCode !== null) throw new Error(`Fixture server exited before becoming ready (code ${server.exitCode})`);
    try {
      const response = await fetch(`${url}/healthz`);
      if (response.ok) return;
      lastError = new Error(`Fixture server health check returned ${response.status}`);
    } catch (error) {
      lastError = error;
    }
    await delay(100);
  }
  throw new Error(`Fixture server did not become ready at ${url}: ${String(lastError)}`);
}

async function stopFixture(server: ChildProcess): Promise<void> {
  if (server.exitCode !== null || server.signalCode !== null) return;
  if (server.pid === undefined) return;
  try {
    process.kill(-server.pid, 'SIGTERM');
  } catch {
    server.kill('SIGTERM');
  }
  const exited = new Promise<void>(resolve => server.once('exit', () => resolve()));
  await Promise.race([exited, delay(5_000)]);
  if (server.exitCode === null && server.signalCode === null) {
    try {
      process.kill(-server.pid, 'SIGKILL');
    } catch {
      server.kill('SIGKILL');
    }
  }
}

type WorkerFixtureServer = {
  forFile: (file: string) => Promise<string>;
  close: () => Promise<void>;
};

export const test = base.extend<{}, { fixtureServer: WorkerFixtureServer }>({
  fixtureServer: [async ({}, use, workerInfo) => {
    const externalURL = process.env.MOUSEION_REUSE_FIXTURE === '1'
      ? process.env.MOUSEION_FIXTURE_URL ?? (process.env.MOUSEION_FIXTURE_ADDR && `http://${process.env.MOUSEION_FIXTURE_ADDR}`)
      : undefined;
    if (process.env.MOUSEION_REUSE_FIXTURE === '1' && !externalURL) {
      throw new Error('Set MOUSEION_FIXTURE_URL or MOUSEION_FIXTURE_ADDR when MOUSEION_REUSE_FIXTURE=1.');
    }
    if (externalURL && workerInfo.config.workers !== 1) {
      throw new Error('External fixture review requires --workers=1 to avoid shared mutable state.');
    }

    let activeFile: string | undefined;
    let activeURL: string | undefined;
    let activeProcess: ChildProcess | undefined;
    const fixtureServer: WorkerFixtureServer = {
      forFile: async file => {
        if (externalURL) {
          if (activeFile && activeFile !== file) {
            throw new Error('External fixture mode supports one spec file per invocation; restart it between files.');
          }
          activeFile = file;
          return externalURL;
        }
        if (activeFile === file && activeURL) return activeURL;
        if (activeProcess) await stopFixture(activeProcess);

        const port = await availableLoopbackPort();
        const addr = `127.0.0.1:${port}`;
        const url = `http://${addr}`;
        const binary = process.env.MOUSEION_FIXTURE_BIN;
        activeProcess = spawn(binary ?? 'go', binary ? [] : ['run', '../cmd/fixtureserver'], {
          cwd: process.cwd(),
          env: { ...process.env, MOUSEION_FIXTURE_ADDR: addr },
          stdio: ['ignore', 'ignore', 'inherit'],
          detached: true,
        });
        await waitForFixture(url, activeProcess);
        activeFile = file;
        activeURL = url;
        return url;
      },
      close: async () => {
        if (activeProcess) await stopFixture(activeProcess);
      },
    };
    try {
      await use(fixtureServer);
    } finally {
      await fixtureServer.close();
    }
  }, { scope: 'worker' }],
  baseURL: async ({ fixtureServer }, use, testInfo) => {
    await use(await fixtureServer.forFile(testInfo.file));
  },
});

export { expect };
