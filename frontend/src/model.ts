// Go API retains exported field names; null slices and pointer fields are expected.
export type Obj = Record<string, any>;
export interface ApiEnvelope { id: string; result: ApiResult }
export interface ApiResult extends Obj { Plan: ApiPlan; Analysis: ApiAnalysis; Reports: ApiReport[] | null }
export interface ApiPlan extends Obj { Status: string; Entries: ApiEntry[] | null; Blockers: Obj[] | null; Diagnostics: Obj[] | null }
export interface ApiEntry extends Obj { Package: Obj; Target: Obj; Installed: Obj; FlagGroups?: Obj[] | null }
export interface ApiAnalysis extends Obj { Status: string; Findings: ApiFinding[] | null }
export interface ApiFinding extends Obj { Message: string; EntryIndex?: number | null; Evidence?: unknown }
export interface ApiReport extends Obj { Report: Obj; BeforeLookup?: ApiLookup; AfterLookup?: ApiLookup }
export interface ApiLookup extends Obj { Query?: Obj | null; Result?: Obj | null; Err?: string | null; SkipReason?: string }

export const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export const array = (value: unknown): any[] => Array.isArray(value) ? value : [];
const object = (value: unknown): value is Obj => value !== null && typeof value === 'object' && !Array.isArray(value);
export function envelope(value: unknown): value is ApiEnvelope {
  if (!object(value) || typeof value.id !== 'string' || !uuid.test(value.id) || !object(value.result)) return false;
  const { Plan, Analysis, Reports } = value.result;
  const objects = (v: unknown) => v === null || (Array.isArray(v) && v.every(object));
  if (!object(Plan) || typeof Plan.Status !== 'string' || !object(Analysis) || typeof Analysis.Status !== 'string') return false;
  if (!['Entries', 'Blockers', 'Diagnostics'].every(k => objects(Plan[k])) || !objects(Analysis.Findings) || !objects(Reports)) return false;
  return array(Plan.Entries).every(entry => object(entry.Package) && object(entry.Target) && object(entry.Installed) && objects(entry.Installed.Versions) && (entry.FlagGroups === undefined || objects(entry.FlagGroups)) && array(entry.FlagGroups).every((group: Obj) => objects(group.Values)))
    && array(Reports).every(view => object(view.Report))
    && array(Analysis.Findings).every(finding => typeof finding.Message === 'string');
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
  const ev = finding.Evidence;
  if (!ev || typeof ev !== 'object') return ['Основание недоступно'];
  const entry = Number.isInteger(finding.EntryIndex) ? array(plan.Entries)[finding.EntryIndex] : undefined;
  const result: string[] = [];
  const show = (name: string, value: unknown) => result.push(`${name}: ${value === undefined || value === null ? 'недоступно' : JSON.stringify(value)}`);
  if (entry) show('Строки', entry.SourceLines);
  if ('ContextIndex' in ev) show('Индекс контекста', ev.ContextIndex);
  if ('BlockerIndex' in ev) show('Blocker', array(plan.Blockers)[ev.BlockerIndex]);
  if ('InstalledVersionIndex' in ev) show('Установленная версия', array(entry?.Installed?.Versions)[ev.InstalledVersionIndex]);
  if ('Tokens' in ev) for (const ref of array(ev.Tokens)) show('USE', object(ref) ? array(array(entry?.FlagGroups)[ref.GroupIndex]?.Values)[ref.ValueIndex] : undefined);
  if (!['ContextIndex', 'BlockerIndex', 'InstalledVersionIndex', 'Tokens'].some(key => key in ev) || 'Operation' in ev) show('Evidence', ev);
  return result;
}
