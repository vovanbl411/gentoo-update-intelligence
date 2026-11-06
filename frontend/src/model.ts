import type { CompactResult, ApiError } from './api-types';
export type { ApiResult, ApiError } from './api-types';
export type Obj = Record<string, any>;
export interface ApiEnvelope { id: string; result: CompactResult }
export interface AnalysisJob {
  id: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  progress: { stage: string; completed: number; total: number };
  result: CompactResult | null;
  error: ApiError | null;
}

export const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export const array = (value: unknown): any[] => Array.isArray(value) ? value : [];
const object = (value: unknown): value is Obj => value !== null && typeof value === 'object' && !Array.isArray(value);
export function envelope(value: unknown): value is ApiEnvelope {
  if (!object(value) || typeof value.id !== 'string' || !uuid.test(value.id) || !object(value.result)) return false;
  const { plan, analysis, reports } = value.result;
  const objects = (v: unknown) => Array.isArray(v) && v.every(object);
  if (!object(plan) || typeof plan.status !== 'string' || !object(analysis) || typeof analysis.status !== 'string') return false;
  if (!['entries', 'blockers', 'removals', 'diagnostics'].every(k => objects(plan[k])) || !objects(analysis.findings) || !objects(analysis.diagnostics) || !objects(reports) || !object(value.result.repositories)) return false;
  return array(plan.entries).every(entry => object(entry.package) && object(entry.target) && object(entry.installed) && objects(entry.installed.versions) && (entry.flag_groups === undefined || objects(entry.flag_groups)) && array(entry.flag_groups).every((group: Obj) => objects(group.values)))
    && array(reports).every(view => object(view.report))
    && array(analysis.findings).every(finding => typeof finding.message === 'string');
}
export function analysisJob(value: unknown): value is AnalysisJob {
  if (!object(value) || typeof value.id !== 'string' || !uuid.test(value.id)) return false;
  if (!['pending', 'running', 'completed', 'failed'].includes(value.status)) return false;
  if (!object(value.progress) || typeof value.progress.stage !== 'string' ||
      !Number.isInteger(value.progress.completed) || value.progress.completed < 0 ||
      !Number.isInteger(value.progress.total) || value.progress.total < 0) return false;
  if (value.status === 'completed') return value.error === null && envelope({ id: value.id, result: value.result });
  if (value.result !== null) return false;
  if (value.status === 'failed') return object(value.error) && typeof value.error.code === 'string' && typeof value.error.message === 'string';
  return value.error === null;
}
export function partialJob(value: unknown): value is AnalysisJob & { pending_entries: number[] } {
  if (!object(value) || !Array.isArray(value.pending_entries) || !value.pending_entries.every((v: unknown) => Number.isInteger(v) && (v as number) >= 0)) return false;
  const base = { ...value, result: value.status === 'completed' ? value.result : null };
  return analysisJob(base) && (value.result === null || envelope({ id: value.id, result: value.result }));
}
export function input(plan: string, lines: string) {
  if (!plan.trim()) throw new Error('Введите план.');
  const packages = lines.split(/\r?\n/).map(s => s.trim()).filter(Boolean).map(s => {
    const match = /^([A-Za-z0-9_][A-Za-z0-9+_.-]*)\/([A-Za-z0-9_][A-Za-z0-9+_-]*)$/.exec(s);
    if (!match || /-[0-9]+(\.[0-9]+)*[a-z]?(_(alpha|beta|pre|rc|p)[0-9]*)*(-r[0-9]+)?$/.test(match[2])) throw new Error('Укажите точный category/name в каждой строке.');
    return { category: match[1], name: match[2] };
  });
  return { plan, locally_patched_packages: packages };
}
export function evidence(plan: Obj, finding: Obj): string[] {
  const ev = finding.evidence;
  if (!ev || typeof ev !== 'object') return ['Основание недоступно'];
  const entry = Number.isInteger(finding.entry_index) ? array(plan.entries)[finding.entry_index] : undefined;
  const result: string[] = [];
  const show = (name: string, value: unknown) => result.push(`${name}: ${value === undefined || value === null ? 'недоступно' : JSON.stringify(value)}`);
  if (entry) show('Строки', entry.source_lines);
  if ('context_index' in ev) show('Индекс контекста', ev.context_index);
  if ('blocker_index' in ev) show('Blocker', array(plan.blockers)[ev.blocker_index]);
  if ('installed_version_index' in ev) show('Установленная версия', array(entry?.installed?.versions)[ev.installed_version_index]);
  if ('tokens' in ev) for (const ref of array(ev.tokens)) show('USE', object(ref) ? array(array(entry?.flag_groups)[ref.group_index]?.values)[ref.value_index] : undefined);
  if (!['context_index', 'blocker_index', 'installed_version_index', 'tokens'].some(key => key in ev) || 'operation' in ev) show('Evidence', ev);
  return result;
}
