import { array, envelope, evidence, input, type Obj } from './model';
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
      const response = await fetch('/api/analyses', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
      if (response.status !== 201) { let explanation = ''; try { const data = await response.json(); explanation = typeof data.error === 'string' ? `: ${data.error}` : ''; } catch {} status.textContent = `${response.status === 503 ? 'Сервис недоступен' : 'Ошибка запроса'} (${response.status})${explanation}`; return; }
      let data: unknown; try { data = await response.json(); } catch { status.textContent = 'Некорректный ответ: сохранение не подтверждено.'; return; }
      if (!envelope(data)) { status.textContent = 'Некорректный ответ: сохранение не подтверждено.'; return; }
      navigate(`/analyses/${data.id}`);
    } catch { status.textContent = 'Ошибка сети: сохранение не подтверждено. Повторная отправка может создать второй анализ.'; }
    finally { pending = false; button.disabled = false; }
  };
}
function report(parent: HTMLElement, view: Obj) {
  const r = view.Report; if (!r) return; const section = el('section'); add(section, 'h3', 'Metadata');
  const g = r.Gentoo; if (g) { add(section, 'p', `Сравнение: ${g.Status}`); const c = g.Comparison;
    if (c) { add(section, 'p', `Сравнённые поля: ${array(c.ComparedFields).join(', ') || 'не указаны'}`); const changes = c.Changes; const i = changes?.IUSE; if (array(c.ComparedFields).length && changes && (!array(c.ComparedFields).includes('IUSE') || (i && ['Added', 'Removed', 'DefaultChanges'].every(key => Array.isArray(i[key])))) && !changes.EAPI && !changes.Slot && !changes.Subslot && !array(i?.Added).length && !array(i?.Removed).length && !array(i?.DefaultChanges).length) add(section, 'p', 'В сравнённых полях различий нет.'); detail(section, 'Изменения', changes); detail(section, 'Ограничения сравнения', c.Limitations); detail(section, 'Provenance', { Before: c.Before, After: c.After }); }
    detail(section, 'Диагностики связи', g.Diagnostics);
  }
  for (const [name, lookup] of [['before', view.BeforeLookup], ['after', view.AfterLookup]] as const) {
    const reasons = [lookup?.Result?.Status, lookup?.SkipReason, lookup?.Err].filter(value => typeof value === 'string' && value.length);
    add(section, 'p', `${name}: ${reasons.length ? reasons.join('; ') : 'данные недоступны'}`); detail(section, `${name}: сведения Lookup`, lookup);
  }
  for (const [name, auxiliary] of [['EffectiveConfig', r.EffectiveConfig], ['Upstream', r.Upstream]] as const) {
    if (auxiliary) add(section, 'p', `${name}: ${[auxiliary.Status, auxiliary.Reason].filter(Boolean).join('; ') || 'данные недоступны'}`);
  }
  if (r.InterpretationLimitation) add(section, 'p', r.InterpretationLimitation); parent.append(section);
}
function result(data: Obj, app: HTMLElement) {
  const { Plan: plan, Analysis: analysis, Reports: reports } = data; add(app, 'h1', 'Сохранённый анализ'); link(app, 'Новый анализ', '/');
  add(app, 'p', `Статус Parser: ${plan.Status}`); for (const d of array(plan.Diagnostics)) add(app, 'p', `${d.Code}: ${d.Message} ${array(d.SourceLines).join(', ')}`);
  const findings = array(analysis.Findings); const entries = array(plan.Entries);
  const showFinding = (parent: HTMLElement, f: Obj) => { const item = el('article'); add(item, 'h4', `${f.Category}: ${f.Message}`); add(item, 'p', `Правило: ${f.RuleID}`); for (const line of evidence(plan, f)) add(item, 'p', line); parent.append(item); };
  entries.forEach((entry, index) => { const section = el('section'); add(section, 'h2', `${entry.Package?.Category}/${entry.Package?.Name}`); add(section, 'p', `${entry.Operation}: ${array(entry.Installed?.Versions).map(v => v.Version).join(', ') || 'установленная версия не указана'} → ${entry.Target?.Version || 'версия не указана'}`); add(section, 'p', `Строки: ${array(entry.SourceLines).join(', ') || 'не указаны'}`); findings.filter(f => f.EntryIndex === index).forEach(f => showFinding(section, f)); array(reports).filter(r => r.Report?.EntryIndex === index).forEach(r => report(section, r)); app.append(section); });
  const others = findings.filter(f => !Number.isInteger(f.EntryIndex) || f.EntryIndex < 0 || f.EntryIndex >= entries.length); if (others.length) { add(app, 'h2', 'Другие findings'); others.forEach(f => showFinding(app, f)); }
}
async function load(id: string, current: number) {
  app.replaceChildren(); add(app, 'h1', 'Загрузка анализа'); const state = add(app, 'p', 'Загрузка…'); state.setAttribute('role', 'status');
  const retry = () => { if (current === generation) load(id, current); };
  try { const response = await fetch(`/api/analyses/${encodeURIComponent(id)}`); if (current !== generation) return;
    if (!response.ok) { state.textContent = response.status === 404 ? 'Анализ не найден.' : response.status === 503 ? 'Сервис недоступен.' : `Ошибка загрузки (${response.status}).`; if (response.status === 404) link(app, 'К форме', '/'); else { const b = el('button', 'Повторить загрузку'); b.onclick = retry; app.append(b); } return; }
    const data: unknown = await response.json(); if (current !== generation) return; if (!envelope(data) || data.id.toLowerCase() !== id.toLowerCase()) { state.textContent = 'Некорректный ответ сервиса.'; const b = el('button', 'Повторить загрузку'); b.onclick = retry; app.append(b); return; } const content = el('div'); result(data.result, content); app.replaceChildren(content);
  } catch { if (current !== generation) return; state.textContent = 'Сервис недоступен или ответ некорректен.'; const b = el('button', 'Повторить загрузку'); b.onclick = retry; app.append(b); }
}
function render() { const current = ++generation; app.replaceChildren(); if (location.pathname === '/') form(); else if (location.pathname.startsWith('/analyses/') && location.pathname.slice(10) && !location.pathname.slice(10).includes('/')) void load(location.pathname.slice(10), current); else { add(app, 'h1', 'Страница не найдена'); link(app, 'К форме', '/'); } }
window.addEventListener('popstate', render); render();
