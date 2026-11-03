import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
test('реальный backend и PostgreSQL', async ({ page, context }) => {
  test.skip(process.env.LIVE_ACCEPTANCE !== '1', 'Требуется локальный backend с закреплённой metadata и PostgreSQL');
  const plan = readFileSync('../testdata/parser/ebuild-updates.txt', 'utf8');
  await page.goto('/'); await page.getByLabel('План emerge').fill(plan); await page.getByLabel(/Локально/).fill('dev-libs/libei');
  const responsePromise = page.waitForResponse(r => r.url().endsWith('/api/analyses') && r.request().method() === 'POST');
  await page.getByRole('button', { name: 'Анализировать' }).click();
  const response = await responsePromise; expect(response.status()).toBe(201); const data = await response.json();
  await expect(page).toHaveURL(`/analyses/${data.id}`);
  const get = await page.request.get(`/api/analyses/${data.id}`); expect(await get.json()).toEqual(data);
  const first = data.result.Reports[0]; expect(first.BeforeLookup.Result.Status).toBe('found'); expect(first.AfterLookup.Result.Status).toBe('found');
  expect(first.Report.Gentoo.Comparison.ComparedFields).toEqual(['EAPI','SLOT','IUSE']);
  expect(first.Report.Gentoo.Comparison.Changes.EAPI).toBeNull(); expect(first.Report.Gentoo.Comparison.Changes.Slot).toBeNull(); expect(first.Report.Gentoo.Comparison.Changes.Subslot).toBeNull();
  for (const key of ['Added','Removed','DefaultChanges']) expect(first.Report.Gentoo.Comparison.Changes.IUSE[key]).toEqual([]);
  for (const entry of data.result.Plan.Entries) { const card = page.locator('section').filter({has: page.getByRole('heading', {name:`${entry.Package.Category}/${entry.Package.Name}`, exact:true})}); await expect(card).toContainText(entry.Operation); await expect(card).toContainText(entry.Target.Version); for (const version of entry.Installed.Versions ?? []) await expect(card).toContainText(version.Version); }
  const finding = data.result.Analysis.Findings.find((f: {Evidence?: {ContextIndex?: number}}) => f.Evidence?.ContextIndex === 0);
  expect(finding).toBeTruthy(); expect(finding.EntryIndex).toBe(0);
  await expect(page.getByText(finding.Message, {exact:false})).toBeVisible();
  await expect(page.getByText('Индекс контекста: 0')).toBeVisible();
  for (const item of data.result.Analysis.Findings ?? []) await expect(page.getByText(item.Message, {exact:false})).toBeVisible();
  const check = async (p: typeof page) => { await expect(p.getByText('Сравнённые поля: EAPI, SLOT, IUSE').first()).toBeVisible(); await expect(p.getByText('В сравнённых полях различий нет.').first()).toBeVisible(); };
  await check(page); const other = await context.newPage(); await other.goto(`/analyses/${data.id}`); await check(other); await expect(other.getByText('Индекс контекста: 0')).toBeVisible(); await page.reload(); await check(page); await expect(page.getByText('Индекс контекста: 0')).toBeVisible();
  console.log(`LIVE_ACCEPTANCE_UUID=${data.id}`);
});
