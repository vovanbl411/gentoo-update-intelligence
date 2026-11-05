import fixture from './fixtures/api-found.json';
import { describe, expect, it } from 'vitest';
import { analysisJob, envelope, evidence, input } from '../../src/model';
const id = '12345678-1234-1234-1234-123456789abc';

describe('форма', () => {
  it('сохраняет исходный план и порядок пакетов', () => {
    expect(input(' plan\n', ' a/b \n\n c/d ')).toEqual({ plan: ' plan\n', locally_patched_packages: [{ category: 'a', name: 'b' }, { category: 'c', name: 'd' }] });
  });
  it('проверяет обязательный план и точные пары', () => {
    expect(() => input('  ', '')).toThrow();
    for (const value of ['a', '/b', 'a/b-1.2', 'a/b/c']) expect(() => input('x', value)).toThrow();
    expect(input('x', '\n ').locally_patched_packages).toEqual([]);
  });
});

describe('DTO v1', () => {
  it('принимает fixture и дополнительные поля', () => {
    expect(envelope(fixture)).toBe(true);
    expect(analysisJob({ ...fixture, extra: 1 })).toBe(true);
  });
  it('отклоняет прежний envelope без состояния', () => {
    expect(analysisJob({ id, result: fixture.result })).toBe(false);
  });
  it('отклоняет null вместо списка, неверный UUID и неполный результат', () => {
    expect(envelope({ ...fixture, id: 'x' })).toBe(false);
    expect(envelope({ id, result: {} })).toBe(false);
    const broken = structuredClone(fixture) as any;
    broken.result.plan.entries = null;
    expect(envelope(broken)).toBe(false);
  });
  it('отклоняет повреждённые вложенные элементы', () => {
    for (const field of ['entries', 'blockers', 'diagnostics']) {
      const broken = structuredClone(fixture) as any;
      broken.result.plan[field] = [null];
      expect(envelope(broken)).toBe(false);
    }
    const broken = structuredClone(fixture) as any;
    broken.result.plan.entries[0].installed = null;
    expect(envelope(broken)).toBe(false);
  });
  it('принимает все состояния с явными result и error', () => {
    const progress = { stage: 'queued', completed: 0, total: 0 };
    expect(analysisJob({ id, status: 'pending', progress, result: null, error: null })).toBe(true);
    expect(analysisJob({ id, status: 'running', progress: { stage: 'analyzing', completed: 2, total: 4 }, result: null, error: null })).toBe(true);
    expect(analysisJob(fixture)).toBe(true);
    expect(analysisJob({ id, status: 'failed', progress, result: null, error: { code: 'FUTURE_CODE', message: 'provider unavailable' } })).toBe(true);
  });
  it('отклоняет некорректный прогресс и сочетания состояний', () => {
    const base = { id, status: 'pending', progress: { stage: 'queued', completed: 0, total: 0 }, result: null, error: null };
    expect(analysisJob({ ...base, result: fixture.result })).toBe(false);
    expect(analysisJob({ ...base, error: 'legacy error' })).toBe(false);
    expect(analysisJob({ ...base, status: 'failed' })).toBe(false);
    expect(analysisJob({ ...base, progress: { stage: 'analyzing', completed: -1, total: 1 } })).toBe(false);
    expect(analysisJob({ ...fixture, result: null })).toBe(false);
  });
});

it('сохраняет нулевые индексы и показывает доступное evidence', () => {
  const plan = { entries: [{ source_lines: [4], flag_groups: [{ values: [{ raw_token: 'foo*' }] }], installed: { versions: [{ version: '1' }] } }], blockers: [{ atom: '!a/b' }] };
  expect(evidence(plan, { entry_index: 0, evidence: { tokens: [{ group_index: 0, value_index: 0 }] } }).join()).toContain('foo*');
  expect(evidence(plan, { entry_index: null, evidence: { blocker_index: 0 } }).join()).toContain('!a/b');
  expect(evidence(plan, { entry_index: 0, evidence: { installed_version_index: 0 } }).join()).toContain('1');
  expect(evidence(plan, { entry_index: null, evidence: { installed_version_index: 0 } }).join()).toContain('недоступно');
  expect(evidence(plan, { evidence: { unknown: 'value' } }).join()).toContain('value');
});

it('не теряет finding при неполном evidence', () => {
  expect(evidence({ entries: [] }, { entry_index: 0, evidence: { tokens: [null] } }).join()).toContain('недоступно');
  expect(evidence({}, { evidence: { context_index: null } }).join()).toContain('недоступно');
});
