import { afterEach, expect, it, vi } from 'vitest';
import { createAnalysis, getAnalysis, responseErrorMessage } from '../../src/api';

afterEach(() => vi.unstubAllGlobals());

it('использует только версионированные адреса и сохраняет тело запроса', async () => {
  const fetch = vi.fn().mockResolvedValue(new Response('{}'));
  vi.stubGlobal('fetch', fetch);
  const body = { plan: ' plan ', locally_patched_packages: [{ category: 'a', name: 'b' }] };
  await createAnalysis(body);
  expect(fetch).toHaveBeenCalledWith('/api/v1/analyses', expect.objectContaining({ method: 'POST', body: JSON.stringify(body) }));
  await getAnalysis('id/with space');
  expect(fetch).toHaveBeenLastCalledWith('/api/v1/analyses/id%2Fwith%20space');
});

it('показывает сообщение неизвестного кода ошибки как текст', async () => {
  const message = '<img src=x onerror=alert(1)>';
  expect(await responseErrorMessage(Response.json({ error: { code: 'FUTURE_CODE', message } }))).toBe(message);
});

it('переносит отсутствие или повреждение JSON ошибки', async () => {
  for (const response of [new Response('not json'), Response.json({ error: null }), Response.json({ error: 'old format' }), Response.json({ error: { code: 'CODE', message: 42 } })]) {
    expect(await responseErrorMessage(response)).toBe('');
  }
});
