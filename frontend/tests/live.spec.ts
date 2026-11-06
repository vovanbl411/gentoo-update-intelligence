import { test, expect } from '@playwright/test';

test('реальный frontend, backend и PostgreSQL', async ({ page, context }) => {
  test.skip(process.env.LIVE_ACCEPTANCE !== '1', 'Требуется локальный Compose');
  test.setTimeout(300_000);
  const base = process.env.LIVE_ACCEPTANCE_URL || 'http://127.0.0.1:8081';
  const plan = '[ebuild U] dev-libs/libei-1.6.0-r1::gentoo [1.6.0::gentoo] USE="(systemd) (-elogind) -test" 0 KiB';
  await page.goto(base); await page.getByLabel('План emerge').fill(plan); await page.getByLabel(/Локально/).fill('dev-libs/libei');
  const responsePromise = page.waitForResponse(r => r.url().endsWith('/api/v1/analyses') && r.request().method() === 'POST');
  await page.getByRole('button', { name: 'Анализировать' }).click();
  const response = await responsePromise; expect(response.status()).toBe(202); const accepted = await response.json();
  expect(accepted.status).toBe('pending'); await expect(page).toHaveURL(`${base}/analyses/${accepted.id}`);
  await expect.poll(async () => (await (await page.request.get(`${base}/api/v1/analyses/${accepted.id}/report`)).json()).status, {timeout:240_000}).toBe('completed');
  const compact = await (await page.request.get(`${base}/api/v1/analyses/${accepted.id}/report`)).json();
  const full = await (await page.request.get(`${base}/api/v1/analyses/${accepted.id}`)).json();
  expect(compact.id).toBe(accepted.id); expect(compact.result.plan).toEqual(full.result.plan); expect(compact.result.analysis).toEqual(full.result.analysis);
  const entry = compact.result.plan.entries[0]; expect(`${entry.package.category}/${entry.package.name}`).toBe('dev-libs/libei');
  const check = async (p: typeof page) => {
    const card = p.locator('#package-0'); await expect(card.locator('summary')).toContainText('dev-libs/libei');
    await expect(card.locator('summary')).toContainText(entry.target.version); await card.locator('summary').click();
    await expect(card.getByRole('heading', {name: `Metadata: ${compact.result.reports[0].report.gentoo.status}`})).toBeVisible();
    await expect(p.getByText('Индекс контекста: 0').first()).toBeVisible();
  };
  await check(page); const other = await context.newPage(); await other.goto(`${base}/analyses/${accepted.id}`); await check(other);
  await page.reload(); await check(page);
  await page.getByText('Технические данные', {exact:true}).click(); await expect(page.getByRole('link', {name:'Скачать JSON'})).toBeVisible();
  console.log(`LIVE_ACCEPTANCE_UUID=${accepted.id}`);
});
