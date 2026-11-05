import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
test('реальный backend и PostgreSQL', async ({ page, context }) => {
  test.skip(process.env.LIVE_ACCEPTANCE !== '1', 'Требуется локальный backend с закреплённой metadata и PostgreSQL');
  test.setTimeout(5 * 60 * 1000);
  const plan = readFileSync('../testdata/parser/ebuild-updates.txt', 'utf8');
  await page.goto('/'); await page.getByLabel('План emerge').fill(plan); await page.getByLabel(/Локально/).fill('dev-libs/libei');
  const responsePromise = page.waitForResponse(r => r.url().endsWith('/api/v1/analyses') && r.request().method() === 'POST');
  await page.getByRole('button', { name: 'Анализировать' }).click();
  const response = await responsePromise; expect(response.status()).toBe(202); const accepted = await response.json();
  expect(accepted.status).toBe('pending');
  await expect(page).toHaveURL(`/analyses/${accepted.id}`);
  await expect.poll(async () => (await (await page.request.get(`/api/v1/analyses/${accepted.id}`)).json()).status, {timeout:300_000}).toBe('completed');
  const get = await page.request.get(`/api/v1/analyses/${accepted.id}`); const data = await get.json();
  expect(data.id).toBe(accepted.id); expect(data.result).toBeTruthy();
  const first = data.result.reports[0]; expect(first.before_lookup.result.status).toBe('found'); expect(first.after_lookup.result.status).toBe('found');
  expect(first.report.gentoo.comparison.compared_fields).toEqual(['EAPI','SLOT','IUSE']);
  expect(first.report.gentoo.comparison.changes.eapi).toBeNull(); expect(first.report.gentoo.comparison.changes.slot).toBeNull(); expect(first.report.gentoo.comparison.changes.subslot).toBeNull();
  for (const key of ['added','removed','default_changes']) expect(first.report.gentoo.comparison.changes.iuse[key]).toEqual([]);
  for (const entry of data.result.plan.entries) { const card = page.locator('section').filter({has: page.getByRole('heading', {name:`${entry.package.category}/${entry.package.name}`, exact:true})}); await expect(card).toContainText(entry.operation); await expect(card).toContainText(entry.target.version); for (const version of entry.installed.versions ?? []) await expect(card).toContainText(version.version); }
  const finding = data.result.analysis.findings.find((f: {evidence?: {context_index?: number}}) => f.evidence?.context_index === 0);
  expect(finding).toBeTruthy(); expect(finding.entry_index).toBe(0);
  await expect(page.getByText(finding.message, {exact:false})).toBeVisible();
  await expect(page.getByText('Индекс контекста: 0')).toBeVisible();
  for (const item of data.result.analysis.findings ?? []) await expect(page.getByText(item.message, {exact:false})).toBeVisible();
  const check = async (p: typeof page) => { await expect(p.getByText('Сравнённые поля: EAPI, SLOT, IUSE').first()).toBeVisible(); await expect(p.getByText('В сравнённых полях различий нет.').first()).toBeVisible(); };
  await check(page); const other = await context.newPage(); await other.goto(`/analyses/${data.id}`); await check(other); await expect(other.getByText('Индекс контекста: 0')).toBeVisible(); await page.reload(); await check(page); await expect(page.getByText('Индекс контекста: 0')).toBeVisible();
  console.log(`LIVE_ACCEPTANCE_UUID=${accepted.id}`);
});
