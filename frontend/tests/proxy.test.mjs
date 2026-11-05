import assert from 'node:assert/strict';
import { createServer as createHTTPServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer as createViteServer } from 'vite';

const root = fileURLToPath(new URL('../', import.meta.url));
const id = '12345678-1234-1234-1234-123456789abc';
const path = `/api/v1/analyses/${id}`;
const forwarded = [];
const backend = createHTTPServer((request, response) => {
  forwarded.push(request.url);
  response.setHeader('Content-Type', 'application/json; charset=utf-8');
  if (request.url === '/api/v1/analyses' && request.method === 'POST') {
    response.statusCode = 202;
    response.setHeader('Location', path);
    response.end(JSON.stringify({ id, status: 'pending', progress: { stage: 'queued', completed: 0, total: 0 }, result: null, error: null }));
  } else if (request.url === path) {
    response.end(JSON.stringify({ id, status: 'running', progress: { stage: 'preparing', completed: 0, total: 0 }, result: null, error: null }));
  } else {
    response.statusCode = 404;
    response.end(JSON.stringify({ error: { code: 'NOT_FOUND', message: 'API endpoint not found' } }));
  }
});
await new Promise((resolve, reject) => {
  backend.once('error', reject);
  backend.listen(0, '127.0.0.1', resolve);
});
process.env.GENTOO_UPDATE_INTELLIGENCE_API_PROXY_TARGET = `http://127.0.0.1:${backend.address().port}`;
let vite;
try {
  vite = await createViteServer({ root, configFile: `${root}vite.config.ts`, server: { host: '127.0.0.1', port: 0, open: false } });
  await vite.listen();
  const origin = `http://127.0.0.1:${vite.httpServer.address().port}`;
  const created = await fetch(`${origin}/api/v1/analyses`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ plan: 'test' }) });
  assert.equal(created.status, 202);
  assert.equal(created.headers.get('location'), path);
  assert.match(created.headers.get('content-type'), /^application\/json/);
  assert.equal((await created.json()).id, id);
  assert.equal((await (await fetch(origin + path)).json()).status, 'running');
  for (const unknown of ['/api/analyses', '/api/v2/analyses', '/api/unknown']) {
    const response = await fetch(origin + unknown);
    assert.equal(response.status, 404);
    assert.match(response.headers.get('content-type'), /^application\/json/);
    assert.equal((await response.json()).error.code, 'NOT_FOUND');
  }
  const page = await fetch(`${origin}/analyses/${id}`);
  assert.equal(page.status, 200);
  assert.match(page.headers.get('content-type'), /^text\/html/);
  assert.deepEqual(forwarded, ['/api/v1/analyses', path, '/api/analyses', '/api/v2/analyses', '/api/unknown']);
  // Static guard for the production proxy; the Vite checks above use real HTTP.
  const nginx = await readFile(`${root}nginx.conf`, 'utf8');
  assert.match(nginx, /location \/api\//);
  assert.match(nginx, /proxy_pass http:\/\/backend:8080;/);
  assert.doesNotMatch(nginx, /proxy_pass http:\/\/backend:8080\//);
  assert.match(nginx, /try_files \$uri \$uri\/ \/index\.html;/);
  console.log('PASS: Vite forwards v1 paths, statuses, Location and JSON errors; SPA links work; nginx preserves URI in configuration');
} finally {
  await vite?.close();
  await new Promise((resolve, reject) => backend.close(error => error ? reject(error) : resolve()));
}
