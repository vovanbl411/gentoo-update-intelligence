import { createAnalysis, getAnalysis, responseErrorMessage } from './api';
import { analysisJob, array, evidence, input, type Obj } from './model';
import './style.css';
const app = document.querySelector<HTMLElement>('#app')!;
let generation = 0;
function el<K extends keyof HTMLElementTagNameMap>(tag: K, text?: string): HTMLElementTagNameMap[K] { const node = document.createElement(tag); if (text !== undefined) node.textContent = text; return node; }
function add(parent: HTMLElement, tag: keyof HTMLElementTagNameMap, text: string) { const node = el(tag, text); parent.append(node); return node; }
function detail(parent: HTMLElement, title: string, data: unknown) { const d = el('details'); d.append(el('summary', title), el('pre', JSON.stringify(data, null, 2))); parent.append(d); }
function navigate(path: string) { history.pushState(null, '', path); render(); }
function link(parent: HTMLElement, text: string, path: string) { const a = el('a', text); a.href = path; a.onclick = e => { if (!e.ctrlKey && !e.metaKey && !e.shiftKey && e.button === 0) { e.preventDefault(); navigate(path); } }; parent.append(a); }
function form() {
  add(app, 'h1', 'Анализ плана обновления');
  const f = el('form'); const planLabel = el('label', 'План emerge'); const plan = el('textarea'); plan.rows = 12; planLabel.append(plan);
  const pkgLabel = el('label', 'Локально патченные пакеты (category/name, по одному на строку)'); const pkgs = el('textarea'); pkgs.rows = 4; pkgLabel.append(pkgs);
  const planErr = el('p'); const pkgErr = el('p'); const status = el('p'); status.setAttribute('role', 'status'); const button = el('button', 'Анализировать'); button.type = 'submit';
  f.append(planLabel, planErr, pkgLabel, pkgErr, button, status); app.append(f); let pending = false;
  f.onsubmit = async e => {
    e.preventDefault(); if (pending) return; planErr.textContent = ''; pkgErr.textContent = ''; status.textContent = '';
    let body; try { body = input(plan.value, pkgs.value); } catch (err) { const message = (err as Error).message; (plan.value.trim() ? pkgErr : planErr).textContent = message; return; }
    pending = true; button.disabled = true; status.textContent = 'Отправка…';
    try {
      const response = await createAnalysis(body);
      if (response.status !== 202) { const message = await responseErrorMessage(response); const explanation = message ? `: ${message}` : ''; status.textContent = `${response.status === 503 ? 'Сервис недоступен' : 'Ошибка запроса'} (${response.status})${explanation}`; return; }
      let data: unknown; try { data = await response.json(); } catch { status.textContent = 'Некорректный ответ: сохранение не подтверждено.'; return; }
      if (!analysisJob(data) || !('status' in data) || !['pending', 'running'].includes(data.status)) { status.textContent = 'Некорректный ответ: сохранение не подтверждено.'; return; }
      navigate(`/analyses/${data.id}`);
    } catch { status.textContent = 'Ошибка сети: сохранение не подтверждено. Повторная отправка может создать второй анализ.'; }
    finally { pending = false; button.disabled = false; }
  };
}
function report(parent: HTMLElement, view: Obj) {
  const r = view.report; if (!r) return; const section = el('section'); add(section, 'h3', 'Metadata');
  const g = r.gentoo; if (g) { add(section, 'p', `Сравнение: ${g.status}`); const c = g.comparison;
    if (c) { add(section, 'p', `Сравнённые поля: ${array(c.compared_fields).join(', ') || 'не указаны'}`); const changes = c.changes; const i = changes?.iuse; if (array(c.compared_fields).length && changes && (!array(c.compared_fields).includes('IUSE') || (i && ['added', 'removed', 'default_changes'].every(key => Array.isArray(i[key])))) && !changes.eapi && !changes.slot && !changes.subslot && !array(i?.added).length && !array(i?.removed).length && !array(i?.default_changes).length) add(section, 'p', 'В сравнённых полях различий нет.'); detail(section, 'Изменения', changes); detail(section, 'Ограничения сравнения', c.limitations); detail(section, 'Provenance', { before: c.before, after: c.after }); }
    detail(section, 'Диагностики связи', g.diagnostics);
  }
  for (const [name, lookup] of [['before', view.before_lookup], ['after', view.after_lookup]] as const) {
    const reasons = [lookup?.result?.status, lookup?.skip_reason, lookup?.error?.message].filter(value => typeof value === 'string' && value.length);
    add(section, 'p', `${name}: ${reasons.length ? reasons.join('; ') : 'данные недоступны'}`); detail(section, `${name}: сведения Lookup`, lookup);
  }
  for (const [name, auxiliary] of [['EffectiveConfig', r.effective_config], ['Upstream', r.upstream]] as const) {
    if (auxiliary) add(section, 'p', `${name}: ${[auxiliary.status, auxiliary.reason].filter(Boolean).join('; ') || 'данные недоступны'}`);
  }
  if (r.interpretation_limitation) add(section, 'p', r.interpretation_limitation); parent.append(section);
}
function result(data: Obj, app: HTMLElement) {
  const { plan: plan, analysis: analysis, reports: reports } = data; add(app, 'h1', 'Сохранённый анализ'); link(app, 'Новый анализ', '/');
  add(app, 'p', `Статус Parser: ${plan.status}`); for (const d of array(plan.diagnostics)) add(app, 'p', `${d.code}: ${d.message} ${array(d.source_lines).join(', ')}`);
  const findings = array(analysis.findings); const entries = array(plan.entries);
  const showFinding = (parent: HTMLElement, f: Obj) => { const item = el('article'); add(item, 'h4', `${f.category}: ${f.message}`); add(item, 'p', `Правило: ${f.rule_id}`); for (const line of evidence(plan, f)) add(item, 'p', line); parent.append(item); };
  entries.forEach((entry, index) => { const section = el('section'); add(section, 'h2', `${entry.package?.category}/${entry.package?.name}`); add(section, 'p', `${entry.operation}: ${array(entry.installed?.versions).map(v => v.version).join(', ') || 'установленная версия не указана'} → ${entry.target?.version || 'версия не указана'}`); add(section, 'p', `Строки: ${array(entry.source_lines).join(', ') || 'не указаны'}`); findings.filter(f => f.entry_index === index).forEach(f => showFinding(section, f)); array(reports).filter(r => r.report?.entry_index === index).forEach(r => report(section, r)); app.append(section); });
  const others = findings.filter(f => !Number.isInteger(f.entry_index) || f.entry_index < 0 || f.entry_index >= entries.length); if (others.length) { add(app, 'h2', 'Другие findings'); others.forEach(f => showFinding(app, f)); }
}
async function load(id: string, current: number) {
  app.replaceChildren(); add(app, 'h1', 'Анализ'); link(app, 'Новый анализ', '/');
  const state = add(app, 'p', 'Загрузка состояния…'); state.setAttribute('role', 'status');
  const retry = () => { if (current === generation) void load(id, current); };
  const stageNames: Record<string, string> = { queued: 'В очереди', preparing: 'Подготовка', analyzing: 'Анализ', completed: 'Завершён', failed: 'Ошибка' };
  const showRetry = () => { const button = el('button', 'Повторить загрузку'); button.onclick = retry; app.append(button); };
  const poll = async () => {
    try {
      const response = await getAnalysis(id);
      if (current !== generation) return;
      if (!response.ok) {
        state.textContent = response.status === 404 ? 'Анализ не найден.' : response.status === 503 ? 'Сервис недоступен.' : `Ошибка загрузки (${response.status}).`;
        const explanation = await responseErrorMessage(response);
        if (current !== generation) return;
        if (explanation) state.textContent += ` ${explanation}`;
        if (response.status === 404) link(app, 'К форме', '/'); else showRetry();
        return;
      }
      const data: unknown = await response.json();
      if (current !== generation) return;
      if (!analysisJob(data) || data.id.toLowerCase() !== id.toLowerCase()) { state.textContent = 'Некорректный ответ сервиса.'; showRetry(); return; }
      const progress = data.progress;
      const stage = stageNames[progress.stage] || progress.stage;
      if (data.status === 'completed') {
        if (!data.result) { state.textContent = 'Сервис сообщил о завершении без итогового отчёта.'; showRetry(); return; }
        const content = el('div'); result(data.result, content); app.replaceChildren(content); return;
      }
      if (data.status === 'failed') { state.textContent = `Анализ завершился с ошибкой: ${data.error?.message || 'причина не указана'}`; return; }
      state.textContent = `${data.status === 'pending' ? 'Ожидание' : 'Выполняется'}: ${stage}. Обработано ${progress.completed}${progress.total > 0 ? ` из ${progress.total}` : ''}.`;
      window.setTimeout(() => { if (current === generation) void poll(); }, 1000);
    } catch {
      if (current !== generation) return;
      state.textContent = 'Сервис недоступен или ответ некорректен.'; showRetry();
    }
  };
  void poll();
}
function render() { const current = ++generation; app.replaceChildren(); if (location.pathname === '/') form(); else if (location.pathname.startsWith('/analyses/') && location.pathname.slice(10) && !location.pathname.slice(10).includes('/')) void load(location.pathname.slice(10), current); else { add(app, 'h1', 'Страница не найдена'); link(app, 'К форме', '/'); } }
window.addEventListener('popstate', render); render();
