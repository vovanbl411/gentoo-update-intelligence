import { createAnalysis, getPartialReport, responseErrorMessage } from './api';
import { renderReport } from './report';
import { analysisJob, partialJob, input } from './model';
import './style.css';
const app = document.querySelector<HTMLElement>('#app')!;
let generation = 0;
function el<K extends keyof HTMLElementTagNameMap>(tag: K, text?: string): HTMLElementTagNameMap[K] { const node = document.createElement(tag); if (text !== undefined) node.textContent = text; return node; }
function add(parent: HTMLElement, tag: keyof HTMLElementTagNameMap, text: string) { const node = el(tag, text); parent.append(node); return node; }
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
async function load(id: string, current: number) {
  app.replaceChildren(); const header = el('div'); add(header, 'h1', 'Анализ'); link(header, 'Новый анализ', '/'); app.append(header);
  const state = add(app, 'p', 'Загрузка состояния…'); state.setAttribute('role', 'status');
  const content = el('div'); app.append(content); let lastReport = '';
  const retry = () => { if (current === generation) void load(id, current); };
  const stageNames: Record<string, string> = { queued: 'В очереди', preparing: 'Подготовка', analyzing: 'Анализ', completed: 'Завершён', failed: 'Ошибка' };
  const showRetry = () => { const button = el('button', 'Повторить загрузку'); button.onclick = retry; app.append(button); };
  const poll = async () => {
    try {
      const response = await getPartialReport(id);
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
      if (!partialJob(data) || data.id.toLowerCase() !== id.toLowerCase()) { state.textContent = 'Некорректный ответ сервиса.'; showRetry(); return; }
      const progress = data.progress;
      const stage = stageNames[progress.stage] || progress.stage;
      if (data.result) {
        const key = JSON.stringify([data.result, data.pending_entries, data.status === 'failed']);
        if (key !== lastReport) {
          const search = content.querySelector<HTMLInputElement>('input[type="search"]')?.value ?? '';
          const filter = content.querySelector<HTMLSelectElement>('select')?.value ?? 'all';
          const open = Array.from(content.querySelectorAll<HTMLDetailsElement>('details[id^="package-"][open]'), node => node.id);
          content.replaceChildren(); renderReport(data.result, content, id, data.pending_entries, data.status === 'failed');
          const searchInput = content.querySelector<HTMLInputElement>('input[type="search"]');
          const filterInput = content.querySelector<HTMLSelectElement>('select');
          if (searchInput) { searchInput.value = search; searchInput.dispatchEvent(new Event('input')); }
          if (filterInput) { filterInput.value = filter; filterInput.dispatchEvent(new Event('change')); }
          for (const key of open) { const details = document.getElementById(key) as HTMLDetailsElement | null; if (details) details.open = true; }
          header.hidden = true; lastReport = key;
        }
      }
      if (data.status === 'completed') {
        if (!data.result) { state.textContent = 'Сервис сообщил о завершении без итогового отчёта.'; showRetry(); return; }
        state.textContent = 'Анализ завершён.'; return;
      }
      if (data.status === 'failed') { state.textContent = `Анализ завершился с ошибкой: ${data.error?.message || 'причина не указана'}`; return; }
      state.textContent = `${data.status === 'pending' ? 'Ожидание' : 'Выполняется'}: ${stage}. Обработано ${progress.completed}${progress.total > 0 ? ` из ${progress.total}` : ''}. Полная подготовка metadata может занять больше 30 секунд.`;
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
