import { getAnalysis } from './api';
import { array, type Obj } from './model';

function node<K extends keyof HTMLElementTagNameMap>(tag: K, text?: string): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag);
  if (text !== undefined) element.textContent = text;
  return element;
}
function add(parent: HTMLElement, tag: keyof HTMLElementTagNameMap, text: string) { const item = node(tag, text); parent.append(item); return item; }
function finding(parent: HTMLElement, plan: Obj, f: Obj) {
  const item = node('article'); add(item, 'h4', `${f.category}: ${f.message}`);
  const ev = f.evidence; const entry = array(plan.entries)[f.entry_index];
  if (entry) add(item, 'p', `Строки плана: ${array(entry.source_lines).join(', ') || 'не указаны'}`);
  else if (f.entry_index !== null && f.entry_index !== undefined) add(item, 'p', 'Запись плана недоступна.');
  if (!ev || typeof ev !== 'object') add(item, 'p', 'Основание недоступно.');
  else {
    if ('context_index' in ev) add(item, 'p', `Индекс контекста: ${ev.context_index ?? 'недоступен'}`);
    if ('blocker_index' in ev) { const blocker = array(plan.blockers)[ev.blocker_index]; add(item, 'p', `Blocker: ${blocker?.atom ?? 'недоступен'}`); if (blocker) add(item, 'p', `Строки blocker: ${array(blocker.source_lines).join(', ') || 'не указаны'}`); }
    if ('installed_version_index' in ev) { const installed = array(entry?.installed?.versions)[ev.installed_version_index]; add(item, 'p', `Установленная версия: ${installed?.version ?? 'недоступна'}`); add(item, 'p', `SLOT: ${installed?.slot ?? 'недоступен'} → ${entry?.target?.slot ?? 'недоступен'}`); }
    if ('tokens' in ev) for (const ref of array(ev.tokens)) add(item, 'p', `USE: ${array(array(entry?.flag_groups)[ref?.group_index]?.values)[ref?.value_index]?.raw_token ?? 'недоступен'}`);
    if ('operation' in ev) add(item, 'p', `Операция: ${ev.operation}; установленные версии: ${array(ev.installed_versions).join(', ') || 'не указаны'}; целевая версия: ${ev.target_version || 'не указана'}`);
    if (!['context_index', 'blocker_index', 'installed_version_index', 'tokens', 'operation'].some(key => key in ev)) add(item, 'p', 'Основание недоступно для отображения.');
  }
  add(item, 'p', `Правило: ${f.rule_id}`); parent.append(item);
}
function metadata(parent: HTMLElement, view: Obj) {
  const r = view.report; const g = r?.gentoo; if (!g) return;
  add(parent, 'h4', `Metadata: ${g.status}`); const c = g.comparison;
  if (c) {
    const fields = array(c.compared_fields); add(parent, 'p', `Сравнённые поля: ${fields.join(', ') || 'не указаны'}`);
    const changes = c.changes; const i = changes?.iuse; let count = 0;
    const rows = [['eapi', 'EAPI'], ['slot', 'SLOT'], ['subslot', 'SUBSLOT']].filter(([key]) => changes?.[key]);
    if (rows.length) {
      const table = node('table'); const header = node('tr');
      for (const title of ['Поле', 'До', 'После']) header.append(node('th', title));
      const head = node('thead'); head.append(header); table.append(head); const body = node('tbody');
      for (const [key, label] of rows) { const row = node('tr'); const heading = node('th', label); heading.scope = 'row'; row.append(heading, node('td', changes[key].before), node('td', changes[key].after)); body.append(row); count++; }
      table.append(body); parent.append(table);
    }
    for (const flag of array(i?.added)) { add(parent, 'p', `IUSE добавлен: ${flag.name} (по умолчанию: ${flag.default})`); count++; }
    for (const flag of array(i?.removed)) { add(parent, 'p', `IUSE удалён: ${flag.name} (по умолчанию: ${flag.default})`); count++; }
    for (const flag of array(i?.default_changes)) { add(parent, 'p', `IUSE ${flag.name}: по умолчанию ${flag.before} → ${flag.after}`); count++; }
    if (!count && fields.length && changes && (!fields.includes('IUSE') || (i && ['added','removed','default_changes'].every(k => Array.isArray(i[k]))))) add(parent, 'p', 'В сравнённых полях различий нет.');
    for (const limit of array(c.limitations)) add(parent, 'p', `Ограничение: ${limit.code}; поле: ${limit.field}; сторона: ${limit.side}`);
  }
  for (const d of array(g.diagnostics)) add(parent, 'p', `Диагностика: ${d.code}; сторона: ${d.side ?? 'не указана'}; поле: ${d.field ?? 'не указано'}${d.expected != null ? `; ожидалось: ${d.expected}` : ''}${d.actual != null ? `; получено: ${d.actual}` : ''}`);
  for (const [side, lookup] of [['before', view.before_lookup], ['after', view.after_lookup]] as const) add(parent, 'p', `${side}: ${[lookup?.result?.status, lookup?.skip_reason, lookup?.error?.message].filter(Boolean).join('; ') || 'данные недоступны'}`);
  for (const [label, value] of [['EffectiveConfig', r.effective_config], ['Upstream', r.upstream]] as const) if (value) add(parent, 'p', `${label}: ${value.status}; ${value.reason}`);
  if (r.interpretation_limitation) add(parent, 'p', r.interpretation_limitation);
}
export function renderReport(data: Obj, root: HTMLElement, id: string, pendingEntries: number[] = [], failed = false) {
  const pending = new Set(pendingEntries);
  const { plan, analysis } = data; const entries = array(plan.entries); const findings = array(analysis.findings); const reports = array(data.reports);
  add(root, 'h1', 'Сохранённый анализ'); const home = node('a', 'Новый анализ'); home.href = '/'; root.append(home);
  add(root, 'p', `Статус Parser: ${plan.status}`); for (const d of array(plan.diagnostics)) add(root, 'p', `${d.code}: ${d.message} ${array(d.source_lines).join(', ')}`);
  add(root, 'p', `Пакетов: ${entries.length}; замечаний REVIEW: ${findings.filter(f => f.category === 'REVIEW').length}; блокировок BLOCKER: ${findings.filter(f => f.category === 'BLOCKER').length}; сравнений metadata: ${reports.filter(r => r.report?.gentoo?.comparison).length} из ${entries.length}.`);
  if (findings.length) { add(root, 'h2', 'Замечания'); for (const f of findings) { const article = node('article'); finding(article, plan, f); if (Number.isInteger(f.entry_index) && entries[f.entry_index]) { const a = node('a', 'К пакету'); a.href = `#package-${f.entry_index}`; a.onclick = () => { search.value = ''; filter.value = 'all'; limit = Math.max(limit, f.entry_index + 1); draw(); const details = document.getElementById(`package-${f.entry_index}`) as HTMLDetailsElement | null; if (details) details.open = true; }; article.append(a); } root.append(article); } }
  add(root, 'h2', 'Пакеты'); const label = node('label', 'Поиск пакета'); const search = node('input'); search.type = 'search'; label.append(search); root.append(label);
  const flabel = node('label', 'Фильтр'); const filter = node('select'); for (const [value, title] of [['all','Все'],['review','Требуют проверки'],['changes','Изменения metadata'],['missing','Неполные или недоступные metadata']]) { const option = node('option', title); option.value = value; filter.append(option); } flabel.append(filter); root.append(flabel);
  const count = node('p'); const list = node('div'); const more = node('button', 'Показать ещё'); root.append(count, list, more); let limit = 50;
  const byEntry = new Map<number, Obj[]>(); for (const r of reports) { const key = r.report?.entry_index; if (!byEntry.has(key)) byEntry.set(key, []); byEntry.get(key)!.push(r); }
  function changed(r: Obj) { const c = r.report?.gentoo?.comparison?.changes; return !!(c?.eapi || c?.slot || c?.subslot || array(c?.iuse?.added).length || array(c?.iuse?.removed).length || array(c?.iuse?.default_changes).length); }
  function draw() {
    list.replaceChildren(); const matching = entries.map((entry, index) => ({entry,index})).filter(({entry,index}) => {
      if (!`${entry.package?.category}/${entry.package?.name}`.toLowerCase().includes(search.value.trim().toLowerCase())) return false;
      const rs = byEntry.get(index) || [];
      if (filter.value === 'review') return findings.some(f => f.entry_index === index && ['REVIEW','BLOCKER'].includes(f.category));
      if (filter.value === 'changes') return rs.some(changed);
      if (filter.value === 'missing') return !pending.has(index) && (!rs.length || rs.some(r => r.report?.gentoo?.status !== 'available'));
      return true;
    });
    count.textContent = `Найдено пакетов: ${matching.length}`; more.hidden = matching.length <= limit;
    for (const {entry,index} of matching.slice(0, limit)) {
      const d = node('details'); d.id = `package-${index}`; const rs = byEntry.get(index) || []; const fs = findings.filter(f => f.entry_index === index);
      d.append(node('summary', `${entry.package?.category}/${entry.package?.name} — ${entry.operation}: ${array(entry.installed?.versions).map(v => v.version).join(', ') || 'версия не указана'} → ${entry.target?.version || 'не указана'}; замечаний: ${fs.length}; metadata: ${pending.has(index) ? (failed ? 'подготовка остановлена' : 'подготавливаются') : rs.map(r => r.report?.gentoo?.status).join(', ') || 'недоступны'}`));
      d.addEventListener('toggle', () => { if (!d.open || d.dataset.loaded) return; d.dataset.loaded = '1'; add(d, 'p', `Строки: ${array(entry.source_lines).join(', ') || 'не указаны'}`); fs.forEach(f => finding(d, plan, f)); rs.forEach(r => metadata(d, r)); }); list.append(d);
    }
  }
  search.oninput = filter.onchange = () => { limit = 50; draw(); }; more.onclick = () => { limit += 50; draw(); }; draw();
  const tech = node('details'); tech.append(node('summary', 'Технические данные')); const status = node('p'); tech.append(status); root.append(tech); let loaded = false;
  tech.addEventListener('toggle', async () => { if (!tech.open || loaded) return; loaded = true; status.textContent = 'Загрузка…'; try { const response = await getAnalysis(id); if (!response.ok) throw new Error(`HTTP ${response.status}`); const value: unknown = await response.json(); const json = JSON.stringify(value, null, 2); status.textContent = ''; tech.append(node('pre', json)); const link = node('a', 'Скачать JSON'); link.href = URL.createObjectURL(new Blob([json], {type:'application/json'})); link.download = `analysis-${id}.json`; tech.append(link); } catch { status.textContent = 'Не удалось загрузить технические данные. Закройте и откройте раздел для повтора.'; loaded = false; } });
}
