import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
const apiFixture = JSON.parse(readFileSync(new URL('./unit/fixtures/api-found.json', import.meta.url), 'utf8'));
const id = '12345678-1234-1234-1234-123456789abc';
const accepted = {id,status:'pending',progress:{stage:'queued',completed:0,total:0}};
const result = { plan: { status:'partial', entries:[], blockers:[], diagnostics:[{code:'TEST',message:'<img src=x onerror=alert(1)>',source_lines:[2]}] }, analysis:{status:'partial',findings:[]},reports:[] };
test('POST, GET, прямой вход и refresh', async ({page}) => {
 let posts = 0; await page.route(/\/api\/v1\/analyses(?:\/.*)?$/, async route => { if (route.request().method() === 'POST') { posts++; expect(route.request().postDataJSON()).toEqual({plan:' plan ',locally_patched_packages:[{category:'dev-libs',name:'libei'}]}); await route.fulfill({status:202,json:v1Fixture(accepted)}); } else await route.fulfill({json:v1Fixture({id,result})}); });
 await page.goto('/'); await page.getByLabel('План emerge').fill(' plan '); await page.getByLabel(/Локально/).fill('dev-libs/libei'); await page.getByRole('button', {name:'Анализировать'}).click(); await expect(page).toHaveURL(`/analyses/${id}`); await expect(page.getByText('Статус Parser: partial')).toBeVisible(); expect(posts).toBe(1); await page.reload(); await expect(page.getByText('Статус Parser: partial')).toBeVisible(); await expect(page.locator('img')).toHaveCount(0);
});
for (const status of [400,413,415,500,503]) test(`ошибка ${status} сохраняет ввод`, async ({page}) => { let count=0; await page.route('**/api/v1/analyses', async route => {count++; await route.fulfill({status,json:v1Fixture({error:'ошибка'})})}); await page.goto('/'); await page.getByLabel('План emerge').fill('plan'); await page.getByLabel(/Локально/).fill('dev-libs/libei'); await page.getByRole('button',{name:'Анализировать'}).click(); await expect(page.getByRole('status')).toContainText(String(status)); await expect(page.getByLabel('План emerge')).toHaveValue('plan'); await expect(page.getByLabel(/Локально/)).toHaveValue('dev-libs/libei'); expect(count).toBe(1); });
test('ошибка GET и повтор', async ({page}) => { let count=0; await page.route('**/api/v1/analyses/**', async route => {count++; await route.fulfill(count === 1 ? {status:503} : {json:v1Fixture({id,result})});}); await page.goto(`/analyses/${id}`); await expect(page.getByRole('status')).toContainText('недоступен'); await page.getByRole('button',{name:'Повторить загрузку'}).click(); await expect(page.getByText('Статус Parser: partial')).toBeVisible(); });
test('некорректный вложенный элемент показывает ошибку и повтор', async ({page}) => {
 await page.route('**/api/v1/analyses/**', route => route.fulfill({json:v1Fixture({id,result:{...result,plan:{...result.plan,entries:[null]}}})}));
 await page.goto(`/analyses/${id}`); await expect(page.getByRole('status')).toContainText('Некорректный ответ'); await expect(page.getByRole('button',{name:'Повторить загрузку'})).toBeVisible();
});
test('видны все причины недоступности', async ({page}) => {
 const entry = {package:{category:'dev-libs',name:'libei'},operation:'upgrade',target:{version:'1'},installed:{versions:[]},source_lines:[1]};
 const report = {report:{entry_index:0,gentoo:{status:'unavailable',comparison:null,diagnostics:[]},effective_config:{status:'unavailable',reason:'CONFIG_REASON'},upstream:{status:'unavailable',reason:'UPSTREAM_REASON'}},before_lookup:{result:{status:'not_found'},error:{code:'METADATA_LOOKUP_FAILED',message:'BEFORE_ERROR'},skip_reason:'BEFORE_SKIP'},after_lookup:{result:{status:'unavailable'},error:{code:'METADATA_LOOKUP_FAILED',message:'AFTER_ERROR'},skip_reason:'AFTER_SKIP'}};
 await page.route('**/api/v1/analyses/**', route => route.fulfill({json:v1Fixture({id,result:{...result,plan:{...result.plan,entries:[entry]},reports:[report]}})}));
 await page.goto(`/analyses/${id}`); for (const text of ['CONFIG_REASON','UPSTREAM_REASON','before: not_found','BEFORE_ERROR','BEFORE_SKIP','after: unavailable','AFTER_ERROR','AFTER_SKIP']) await expect(page.getByText(text,{exact:false}).first()).toBeVisible();
});
test('непустой metadata diff и evidence доступны без собственных выводов', async ({page}) => {
 const entry = {package:{category:'dev-libs',name:'libei'},operation:'upgrade',target:{version:'2'},installed:{versions:[{version:'1'}]},flag_groups:[{name:'USE',values:[{raw_token:'foo*'}]}],source_lines:[9]};
 const finding = {category:'review',message:'Изменён USE',rule_id:'USE_CHANGED',entry_index:0,evidence:{tokens:[{group_index:0,value_index:0}]}};
 const comparison = {status:'available',compared_fields:['EAPI','SLOT','IUSE'],changes:{eapi:{before:'7',after:'8'},slot:{before:'0',after:'1'},subslot:null,iuse:{added:[{name:'flag'}],removed:[],default_changes:[]}},limitations:[]};
 const report = {report:{entry_index:0,gentoo:{status:'available',comparison:comparison,diagnostics:[]}},before_lookup:{result:{status:'found'}},after_lookup:{result:{status:'found'}}};
 await page.route('**/api/v1/analyses/**', route => route.fulfill({json:v1Fixture({id,result:{plan:{status:'parsed',entries:[entry],blockers:[],diagnostics:[]},analysis:{status:'parsed',findings:[finding]},reports:[report]}})}));
 await page.goto(`/analyses/${id}`); await expect(page.getByText('foo*',{exact:false})).toBeVisible(); await expect(page.getByText('Строки: [9]')).toBeVisible(); await page.getByText('Изменения',{exact:true}).click(); for (const text of ['"eapi"','"slot"','"flag"']) { await expect(page.locator('pre').filter({hasText:text}).first()).toBeVisible(); } await expect(page.getByText('В сравнённых полях различий нет.')).toHaveCount(0);
});
for (const mode of ['network','no-id']) test(`ошибка POST ${mode} сохраняет ввод`, async ({page}) => { let count=0; await page.route('**/api/v1/analyses', route => {count++; return mode === 'network' ? route.abort('failed') : route.fulfill({status:202,json:v1Fixture({status:'pending'})});}); await page.goto('/'); await page.getByLabel('План emerge').fill('plan'); await page.getByLabel(/Локально/).fill('dev-libs/libei'); await page.getByRole('button',{name:'Анализировать'}).click(); await expect(page.getByRole('status')).toContainText(mode === 'network' ? 'сохранение не подтверждено' : 'Некорректный ответ: сохранение не подтверждено'); await expect(page.getByLabel('План emerge')).toHaveValue('plan'); await expect(page.getByLabel(/Локально/)).toHaveValue('dev-libs/libei'); expect(count).toBe(1); });
test('404 и переход к форме', async ({page}) => { await page.route('**/api/v1/analyses/**', route => route.fulfill({status:404})); await page.goto('/analyses/invalid'); await expect(page.getByRole('status')).toContainText('не найден'); await page.getByRole('link',{name:'К форме'}).click(); await expect(page).toHaveURL('/'); });
test('Back и Forward повторно загружают сохранённый результат', async ({page}) => { let gets=0; await page.route(/\/api\/v1\/analyses(?:\/.*)?$/, route => { if (route.request().method()==='POST') return route.fulfill({status:202,json:v1Fixture(accepted)}); gets++; return route.fulfill({json:v1Fixture({id,result})}); }); await page.goto('/'); await page.getByLabel('План emerge').fill('plan'); await page.getByRole('button',{name:'Анализировать'}).click(); await expect(page.getByText('Статус Parser: partial')).toBeVisible(); await page.goBack(); await expect(page.getByLabel('План emerge')).toBeVisible(); await page.goForward(); await expect(page.getByText('Статус Parser: partial')).toBeVisible(); expect(gets).toBe(2); });
test('во время POST кнопка заблокирована', async ({page}) => { let release!:()=>void; const gate=new Promise<void>(resolve=>{release=resolve}); let posts=0; await page.route(/\/api\/v1\/analyses(?:\/.*)?$/, async route => { if(route.request().method()==='POST'){posts++; await gate; await route.fulfill({status:202,json:v1Fixture(accepted)});} else await route.fulfill({json:v1Fixture({id,result})}); }); await page.goto('/'); await page.getByLabel('План emerge').fill('plan'); const button=page.getByRole('button',{name:'Анализировать'}); await button.click(); await expect(button).toBeDisabled(); await expect(page.getByRole('status')).toContainText('Отправка'); release(); await expect(page.getByText('Статус Parser: partial')).toBeVisible(); expect(posts).toBe(1); });
test('GET показывает прогресс, а refresh продолжает тот же UUID до результата', async ({page}) => {
 let gets=0; const requested:string[]=[];
 await page.route('**/api/v1/analyses/**', route => { gets++; requested.push(new URL(route.request().url()).pathname); const job = gets < 3
   ? {id,status:gets === 1 ? 'pending' : 'running',progress:{stage:gets === 1 ? 'queued' : 'analyzing',completed:gets === 1 ? 0 : 2,total:4}}
   : {id,status:'completed',progress:{stage:'completed',completed:4,total:4},result}; return route.fulfill({json:v1Fixture(job)}); });
 await page.goto(`/analyses/${id}`); await expect(page.getByRole('status')).toContainText('В очереди');
 await page.reload(); await expect(page.getByRole('status')).toContainText('Обработано 2 из 4');
 await expect(page.getByText('Статус Parser: partial')).toBeVisible(); expect(gets).toBe(3); expect(new Set(requested)).toEqual(new Set([`/api/v1/analyses/${id}`]));
});
test('ошибка задания показывает причину из API', async ({page}) => {
 await page.route('**/api/v1/analyses/**', route => route.fulfill({json:v1Fixture({id,status:'failed',progress:{stage:'failed',completed:0,total:3},error:'metadata provider is unavailable'})}));
 await page.goto(`/analyses/${id}`); await expect(page.getByRole('status')).toContainText('metadata provider is unavailable');
});
for (const status of ['parsed','partial','unsupported']) test(`статус ${status} и blocker без EntryIndex`, async ({page}) => { const finding={category:'review',message:'Blocker finding',rule_id:'BLOCKER',entry_index:null,evidence:{blocker_index:0}}; const plan={status:status,entries:[],blockers:[{atom:'!dev-libs/libei',source_lines:[7]}],diagnostics:[]}; await page.route('**/api/v1/analyses/**', route=>route.fulfill({json:v1Fixture({id,result:{plan:plan,analysis:{status:status,findings:[finding]},reports:[]}})})); await page.goto(`/analyses/${id}`); await expect(page.getByText(`Статус Parser: ${status}`)).toBeVisible(); await expect(page.getByText('Blocker finding',{exact:false})).toBeVisible(); await expect(page.getByText('!dev-libs/libei',{exact:false})).toBeVisible(); });
test('сетевой сбой GET позволяет повтор', async ({page}) => { let count=0; await page.route('**/api/v1/analyses/**', route=>++count===1?route.abort('failed'):route.fulfill({json:v1Fixture({id,result})})); await page.goto(`/analyses/${id}`); await expect(page.getByRole('status')).toContainText('недоступен'); await page.getByRole('button',{name:'Повторить загрузку'}).click(); await expect(page.getByText('Статус Parser: partial')).toBeVisible(); });
test('форма и повтор загрузки доступны с клавиатуры', async ({page}) => { await page.goto('/'); await page.keyboard.press('Tab'); await expect(page.getByLabel('План emerge')).toBeFocused(); await page.keyboard.press('Tab'); await expect(page.getByLabel(/Локально/)).toBeFocused(); await page.keyboard.press('Tab'); await expect(page.getByRole('button',{name:'Анализировать'})).toBeFocused(); await page.route('**/api/v1/analyses/**',route=>route.fulfill({status:503})); await page.goto(`/analyses/${id}`); await expect(page.getByRole('button',{name:'Повторить загрузку'})).toBeVisible(); await page.keyboard.press('Tab'); await expect(page.getByRole('link',{name:'Новый анализ'})).toBeFocused(); await page.keyboard.press('Tab'); await expect(page.getByRole('button',{name:'Повторить загрузку'})).toBeFocused(); });
test('устаревший GET не подменяет другой результат', async ({page}) => { const second='aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa'; let release!:()=>void; const gate=new Promise<void>(r=>{release=r}); let firstSeen!:()=>void; const started=new Promise<void>(r=>{firstSeen=r}); await page.route('**/api/v1/analyses/**', async route=>{ if(route.request().url().endsWith(id)){firstSeen(); await gate; await route.fulfill({json:v1Fixture({id,result})}).catch(()=>{}); } else await route.fulfill({json:v1Fixture({id:second,result:{...result,plan:{...result.plan,status:'parsed'}}})}); }); await page.goto(`/analyses/${id}`); await started; await page.evaluate((path)=>{history.pushState(null,'',path); window.dispatchEvent(new PopStateEvent('popstate'));}, `/analyses/${second}`); await expect(page.getByText('Статус Parser: parsed')).toBeVisible(); release(); await expect(page.getByText('Статус Parser: parsed')).toBeVisible(); await expect(page.getByText('Статус Parser: partial')).toHaveCount(0); });

for (const scenario of [
 {name:'не найдено before',side:'before',lookup:{query:null,result:{status:'not_found',snapshot:null},error:null,skip_reason:''},expected:'not_found'},
 {name:'repository unavailable after',side:'after',lookup:{query:null,result:{status:'repository_unavailable',snapshot:null},error:null,skip_reason:''},expected:'repository_unavailable'},
 {name:'ошибка before',side:'before',lookup:{query:null,result:null,error:{code:'METADATA_LOOKUP_FAILED',message:'metadata read failed'},skip_reason:''},expected:'metadata read failed'},
 {name:'пропущен after',side:'after',lookup:{query:null,result:null,error:null,skip_reason:'BINDING_REJECTED'},expected:'BINDING_REJECTED'},
 {name:'нет причины before',side:'before',lookup:{query:null,result:null,error:null,skip_reason:''},expected:'данные недоступны'},
]) test(`metadata: ${scenario.name}`, async ({page}) => {
 const fixture = structuredClone(apiFixture);
 const view=fixture.result.reports[0] as any;
 view.report.gentoo={status:'unavailable',comparison:null,diagnostics:[{code:'SNAPSHOT_MISSING',side:scenario.side,field:'snapshot',expected:null,actual:null}]};
 view[scenario.side==='before'?'before_lookup':'after_lookup']=scenario.lookup;
 await page.route('**/api/v1/analyses/**',route=>route.fulfill({json:v1Fixture(fixture)})); await page.goto(`/analyses/${fixture.id}`);
 const card=page.locator('section').filter({has:page.getByRole('heading',{name:'dev-libs/libei',exact:true})});
 await expect(card.getByText(`${scenario.side}: ${scenario.expected}`,{exact:false})).toBeVisible();
 await card.getByText('Диагностики связи',{exact:true}).click(); await expect(card.locator('pre').filter({hasText:'SNAPSHOT_MISSING'})).toContainText(scenario.side);
 await expect(card.getByText('В сравнённых полях различий нет.')).toHaveCount(0);
});
test('metadata: частичное сравнение показывает ограничения и сторону', async ({page}) => {
 const fixture=structuredClone(apiFixture); const gentoo=fixture.result.reports[0].report.gentoo as any;
 gentoo.status='partial'; gentoo.comparison.status='partial'; gentoo.comparison.compared_fields=['EAPI']; gentoo.comparison.limitations=[{code:'IUSE_MISSING',field:'IUSE',side:'after',evidence:[]}];
 await page.route('**/api/v1/analyses/**',route=>route.fulfill({json:v1Fixture(fixture)})); await page.goto(`/analyses/${fixture.id}`); const card=page.locator('section').filter({has:page.getByRole('heading',{name:'dev-libs/libei',exact:true})}); await expect(card.getByText('Сравнение: partial')).toBeVisible(); await card.getByText('Ограничения сравнения',{exact:true}).click(); await expect(card.locator('pre').filter({hasText:'IUSE_MISSING'})).toContainText('after');
});
test('отсутствующие изменения IUSE не считаются пустым diff', async ({page}) => { const fixture=structuredClone(apiFixture); fixture.result.reports[0].report.gentoo.comparison.changes.iuse=null; await page.route('**/api/v1/analyses/**',route=>route.fulfill({json:v1Fixture(fixture)})); await page.goto(`/analyses/${fixture.id}`); const card=page.locator('section').filter({has:page.getByRole('heading',{name:'dev-libs/libei',exact:true})}); await expect(card.getByText('Сравнённые поля: EAPI, SLOT, IUSE')).toBeVisible(); await expect(card.getByText('В сравнённых полях различий нет.')).toHaveCount(0); });

// Builders keep mock responses on the public v1 envelope while allowing each
// scenario to focus on the fields it exercises.
function v1Fixture(value: any): any {
 if (!value || typeof value !== 'object') return value;
 if ('id' in value && ('status' in value || 'result' in value)) {
  const result = value.result == null ? null : {
   ...value.result,
   plan: { removals: [], conflict_summary: null, summary: null, ...value.result.plan },
   analysis: { diagnostics: [], ...value.result.analysis },
   repositories: value.result.repositories ?? {},
  };
  return { ...value, status: value.status ?? 'completed',
   progress: value.progress ?? { stage: 'completed', completed: 1, total: 1 },
   result, error: typeof value.error === 'string'
    ? { code: 'ANALYSIS_FAILED', message: value.error } : value.error ?? null };
 }
 if (typeof value.error === 'string') return { error: { code: 'FUTURE_CODE', message: value.error } };
 return value;
}
