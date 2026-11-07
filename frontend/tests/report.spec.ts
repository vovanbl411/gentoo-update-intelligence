import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';

const fixture = JSON.parse(readFileSync(new URL('./unit/fixtures/api-found.json', import.meta.url), 'utf8'));
fixture.pending_entries = [];

test('промежуточный отчёт показывает ожидание и обновляет metadata', async ({ page }) => {
  const job = structuredClone(fixture) as any;
  const report = job.result.reports[0];
  job.status = 'running'; job.progress = {stage:'analyzing', completed:0, total:2};
  job.result.reports = []; job.pending_entries = [0];
  let calls = 0;
  await page.route('**/api/v1/analyses/**', route => {
    calls++;
    if (calls > 1) { job.status = 'completed'; job.progress = {stage:'completed', completed:2, total:2}; job.result.reports = [report]; job.pending_entries = []; }
    return route.fulfill({json:job});
  });
  await page.goto(`/analyses/${job.id}`);
  await expect(page.locator('#package-0')).toContainText('подготавливаются');
  await expect(page.getByRole('status')).toContainText('Обработано 0 из 2');
  await expect(page.getByRole('status')).toContainText('может занять больше 30 секунд');
  await expect(page.getByRole('status')).toContainText('Анализ завершён');
  await expect(page.locator('#package-0')).not.toContainText('подготавливаются');
});

test('компактный отчёт, фильтр, страницы и технические данные', async ({ page }) => {
  const job = structuredClone(fixture) as any;
  const first = job.result.plan.entries[0];
  job.result.plan.entries = Array.from({ length: 1000 }, (_, index) => ({ ...first, package: { category: 'dev-test', name: `pkg${index}` } }));
  let fullRequests = 0;
  await page.route('**/api/v1/analyses/**', route => {
    if (!route.request().url().endsWith('/partial-report')) fullRequests++;
    return route.fulfill({ json: job });
  });
  await page.goto(`/analyses/${job.id}`);
  await expect(page.getByText('Найдено пакетов: 1000')).toBeVisible();
  await expect(page.locator('details[id^="package-"]')).toHaveCount(50);
  await page.getByRole('button', { name: 'Показать ещё' }).click();
  await expect(page.locator('details[id^="package-"]')).toHaveCount(100);
  await page.getByLabel('Поиск пакета').fill('pkg999');
  await expect(page.getByText('Найдено пакетов: 1')).toBeVisible();
  await expect(page.getByText('dev-test/pkg999', { exact: false })).toBeVisible();
  expect(fullRequests).toBe(0);
  await page.getByText('Технические данные', { exact: true }).click();
  await expect(page.getByRole('link', { name: 'Скачать JSON' })).toBeVisible();
  expect(fullRequests).toBe(1);
});

test('фильтры сочетаются с поиском и сохраняют порядок', async ({ page }) => {
  const job = structuredClone(fixture) as any;
  const entry = job.result.plan.entries[0];
  job.result.plan.entries = ['alpha', 'beta', 'gamma'].map(name => ({ ...entry, package: { category: 'dev-test', name } }));
  job.result.analysis.findings = [{ category: 'REVIEW', message: 'Проверить', rule_id: 'TEST', entry_index: 1, evidence: null }];
  const baseReport = job.result.reports[0];
  job.result.reports = [0, 1, 2].map(index => ({ ...structuredClone(baseReport), report: { ...structuredClone(baseReport.report), entry_index: index } }));
  job.result.reports[0].report.gentoo.status = 'unavailable';
  job.result.reports[1].report.gentoo.status = 'partial';
  job.result.reports[2].report.gentoo.status = 'available';
  job.result.reports[2].report.gentoo.comparison.changes.eapi = { before: '7', after: '8' };
  await page.route('**/api/v1/analyses/**', route => route.fulfill({ json: job }));
  await page.goto(`/analyses/${job.id}`);
  const packages = page.locator('details[id^="package-"]');
  await expect(packages).toHaveCount(3);
  await expect(packages.nth(0)).toContainText('dev-test/alpha');
  await expect(packages.nth(1)).toContainText('dev-test/beta');
  await expect(packages.nth(2)).toContainText('dev-test/gamma');
  await page.getByLabel('Фильтр').selectOption('review');
  await expect(packages).toHaveCount(1); await expect(packages.first()).toContainText('dev-test/beta');
  await page.getByLabel('Поиск пакета').fill('alpha'); await expect(packages).toHaveCount(0);
  await page.getByLabel('Поиск пакета').fill('');
  await page.getByLabel('Фильтр').selectOption('changes'); await expect(packages).toHaveCount(1); await expect(packages.first()).toContainText('dev-test/gamma');
  await page.getByLabel('Фильтр').selectOption('missing'); await expect(packages).toHaveCount(2);
});

test('ошибка технических данных не скрывает отчёт и допускает повтор', async ({ page }) => {
  const job = structuredClone(fixture) as any; let fullRequests = 0;
  await page.route('**/api/v1/analyses/**', route => {
    if (route.request().url().endsWith('/partial-report')) return route.fulfill({ json: job });
    fullRequests++; return route.fulfill(fullRequests === 1 ? { status: 503 } : { json: job });
  });
  await page.goto(`/analyses/${job.id}`);
  const technical = page.getByText('Технические данные', { exact: true });
  await technical.click(); await expect(page.getByText('Не удалось загрузить технические данные.', { exact: false })).toBeVisible();
  await expect(page.getByText('Статус Parser:', { exact: false })).toBeVisible();
  await technical.click(); await technical.click();
  const download = page.waitForEvent('download'); await page.getByRole('link', { name: 'Скачать JSON' }).click();
  expect((await download).suggestedFilename()).toBe(`analysis-${job.id}.json`); expect(fullRequests).toBe(2);
});

test('пять правил и неполные ссылки evidence остаются понятными', async ({ page }) => {
  const job = structuredClone(fixture) as any;
  const entry = job.result.plan.entries[0];
  entry.source_lines = [4]; entry.installed.versions = [{ version: '2', slot: '0' }]; entry.target.slot = '1';
  entry.flag_groups = [{ name: 'USE', values: [{ raw_token: 'foo*' }] }];
  job.result.plan.blockers = [{ atom: '!dev-test/example', source_lines: [7] }];
  job.result.analysis.findings = [
    { rule_id: 'LOCAL_PATCHED_PACKAGE', category: 'REVIEW', message: 'Локальный патч', entry_index: 0, evidence: { context_index: 0 } },
    { rule_id: 'USE_CHANGED', category: 'REVIEW', message: 'Изменён USE', entry_index: 0, evidence: { tokens: [{ group_index: 0, value_index: 0 }, { group_index: 9, value_index: 0 }] } },
    { rule_id: 'SLOT_CHANGED', category: 'REVIEW', message: 'Изменён SLOT', entry_index: 0, evidence: { installed_version_index: 0 } },
    { rule_id: 'DOWNGRADE_PLANNED', category: 'REVIEW', message: 'Понижение', entry_index: 0, evidence: { operation: 'downgrade', installed_versions: ['2', '3'], target_version: '1' } },
    { rule_id: 'UNRESOLVED_BLOCKER', category: 'BLOCKER', message: 'Блокировка', entry_index: null, evidence: { blocker_index: 0 } },
    { rule_id: 'USE_CHANGED', category: 'REVIEW', message: 'Нет записи', entry_index: 99, evidence: { tokens: [{ group_index: 0, value_index: 0 }] } },
  ];
  await page.route('**/api/v1/analyses/**', route => route.fulfill({ json: job }));
  await page.goto(`/analyses/${job.id}`);
  for (const text of ['Индекс контекста: 0', 'USE: foo*', 'USE: недоступен', 'Установленная версия: 2', 'SLOT: 0 → 1', 'установленные версии: 2, 3', 'Blocker: !dev-test/example', 'Строки blocker: 7', 'Запись плана недоступна.']) await expect(page.getByText(text, { exact: false }).first()).toBeVisible();
});

test('таблица metadata и все виды изменений IUSE', async ({ page }) => {
  const job = structuredClone(fixture) as any; const gentoo = job.result.reports[0].report.gentoo;
  gentoo.comparison.changes = { eapi: {before:'7',after:'8'}, slot: {before:'0',after:'1'}, subslot: {before:'2',after:'3'}, iuse: {added:[{name:'new',default:'enabled'}],removed:[{name:'old',default:'disabled'}],default_changes:[{name:'changed',before:'disabled',after:'enabled'}]} };
  await page.route('**/api/v1/analyses/**', route => route.fulfill({ json: job })); await page.goto(`/analyses/${job.id}`);
  const card = page.locator('#package-0'); await card.locator('summary').click();
  for (const [label, before, after] of [['EAPI','7','8'],['SLOT','0','1'],['SUBSLOT','2','3']]) {
    const row = card.getByRole('row').filter({ has: page.getByRole('rowheader', { name: label, exact: true }) });
    await expect(row.getByRole('cell').nth(0)).toHaveText(before); await expect(row.getByRole('cell').nth(1)).toHaveText(after);
  }
  for (const text of ['IUSE добавлен: new', 'IUSE удалён: old', 'IUSE changed: по умолчанию disabled → enabled']) await expect(card.getByText(text, { exact: false })).toBeVisible();
  await expect(card.getByText('В сравнённых полях различий нет.')).toHaveCount(0);
});

test('43 записи нового плана и семь пересборок без выдуманной установленной стороны', async ({ page }) => {
  const text = readFileSync(new URL('../../testdata/parser/ebuild-updates-3.txt', import.meta.url), 'utf8');
  const job = structuredClone(fixture) as any;
  // Browser transport fixture uses only displayed identities. Parser facts and
  // actual Provider queries are independently covered by Go integration tests.
  job.result.plan.entries = text.trim().split('\n').map((line, index) => {
    const match = line.match(/^\[ebuild\s+(U|R)\s*(~)?\s*\]\s+(\S+)\s*(?:\[([^\]]+)\])?/)!;
    const atom = match[3].match(/^(.+)\/([^/]+)-([0-9][^:]*)(?::([^:]+))?::(.+)$/)!;
    return {
      package: {category:atom[1],name:atom[2]}, operation:match[1] === 'R' ? 'reinstall' : 'upgrade',
      target: {version:atom[3],repository:atom[5]}, installed: {versions:match[4] ? [{version:match[4].split(':')[0]}] : []},
      flag_groups: [], source_lines:[index + 1],
    };
  });
  job.result.plan.status = 'parsed'; job.result.plan.diagnostics = []; job.result.plan.summary = null;
  job.result.analysis.findings = []; job.result.analysis.diagnostics = []; job.result.analysis.status = 'parsed';
  const report = structuredClone(job.result.reports[0]);
  report.report.gentoo = {status:'unavailable', comparison:null, diagnostics:[{code:'INSTALLED_CONTEXT_MISSING'}]};
  report.before_lookup = {query:null,result:null,skip_reason:'BINDING_REJECTED',error:null};
  report.after_lookup = {result:{status:'not_found'}};
  job.result.reports = job.result.plan.entries.map((_: unknown, index: number) => ({...structuredClone(report),report:{...structuredClone(report.report),entry_index:index}}));
  await page.route('**/api/v1/analyses/**', route => route.fulfill({json:job}));
  await page.goto('/analyses/' + job.id);
  await expect(page.getByText('Распознано пакетов: 43;', {exact:false})).toBeVisible();
  await expect(page.locator('details[id^="package-"]')).toHaveCount(43);
  await expect(page.getByRole('alert')).toHaveCount(0);
  await expect(page.getByText('Всего пакетов по сводке Portage:', {exact:false})).toHaveCount(0);
  const rebuilds = job.result.plan.entries.filter((entry: any) => entry.operation === 'reinstall');
  expect(rebuilds).toHaveLength(7);
  for (const [index, entry] of job.result.plan.entries.entries()) {
    const card = page.locator('#package-' + index);
    await expect(card.locator('summary')).toContainText(entry.package.category + '/' + entry.package.name);
    await expect(card.locator('summary')).toContainText(entry.target.version);
    if (entry.operation === 'reinstall') {
      await expect(card.locator('summary')).toContainText('Пересборка');
      await card.locator('summary').click();
      await expect(card.getByText('Установленная версия не указана в плане; сравнение до/после недоступно.')).toBeVisible();
    }
  }
  await expect(page.getByText('Сравнение metadata ограничено или недоступно', {exact:false})).toBeVisible();
});

for (const parserStatus of ['partial', 'unsupported']) {
  test('сохранённый ' + parserStatus + ': предупреждение отдельно от metadata', async ({ page }) => {
    const job = structuredClone(fixture) as any;
    job.result.plan.status = parserStatus; job.result.analysis.findings = [];
    job.result.plan.diagnostics = [{code:'UNSUPPORTED_OPERATION',message:'Не поддержано',source_lines:[11]}];
    job.result.plan.summary = {packages:43,upgrades:36}; // Older saved result.
    let posts = 0;
    await page.route('**/api/v1/analyses/**', route => {
      if (route.request().method() === 'POST') posts++;
      return route.fulfill({json:job});
    });
    await page.goto('/analyses/' + job.id);
    await expect(page.getByRole('alert')).toContainText('анализ охватывает только распознанные записи');
    await expect(page.getByRole('alert')).toContainText('Отсутствие замечаний не характеризует весь вход');
    await expect(page.getByText('Распознано пакетов:', {exact:false})).toContainText(String(job.result.plan.entries.length));
    await expect(page.getByText('Всего пакетов по сводке Portage:', {exact:false})).toContainText('43');
    job.result.plan.summary = null;
    await page.reload();
    await expect(page.getByRole('alert')).toBeVisible();
    await expect(page.getByText('Всего пакетов по сводке Portage:', {exact:false})).toHaveCount(0);
    expect(posts).toBe(0);
  });
}
