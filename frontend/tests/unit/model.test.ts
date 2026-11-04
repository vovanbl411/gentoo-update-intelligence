import fixture from './fixtures/api-found.json';
import { describe, expect, it } from 'vitest';
import { analysisJob, envelope, evidence, input } from '../../src/model';
const id = '12345678-1234-1234-1234-123456789abc';
describe('форма', () => {
  it('сохраняет исходный план и порядок пакетов', () => expect(input(' plan\n', ' a/b \n\n c/d ')).toEqual({plan:' plan\n',locally_patched_packages:[{category:'a',name:'b'},{category:'c',name:'d'}]}));
  it('проверяет обязательный план и точные пары', () => { expect(() => input('  ', '')).toThrow(); for (const value of ['a', '/b', 'a/b-1.2', 'a/b/c']) expect(() => input('x', value)).toThrow(); expect(input('x', '\n ').locally_patched_packages).toEqual([]); });
});
describe('ответ API', () => {
  const valid = {id,result:{Plan:{Status:'partial', Entries:null, Blockers:null, Diagnostics:null},Analysis:{Status:'partial',Findings:null},Reports:null}};
  it('принимает null-массивы и дополнительные поля', () => expect(envelope({...valid, extra:1})).toBe(true));
  it('отклоняет неполный envelope', () => { expect(envelope({...valid,id:'x'})).toBe(false); expect(envelope({id,result:{}})).toBe(false); });
  it('разрешает нулевой индекс и обозначает недоступные ссылки', () => { const plan = {Entries:[{SourceLines:[4],FlagGroups:[{Values:[{RawToken:'foo*'}]}],Installed:{Versions:[{Version:'1'}]}}],Blockers:[{Atom:'!a/b'}]}; expect(evidence(plan,{EntryIndex:0,Evidence:{Tokens:[{GroupIndex:0,ValueIndex:0}]}}).join()).toContain('foo*'); expect(evidence(plan,{EntryIndex:null,Evidence:{BlockerIndex:0}}).join()).toContain('!a/b'); expect(evidence(plan,{EntryIndex:null,Evidence:{InstalledVersionIndex:0}}).join()).toContain('недоступно'); expect(evidence(plan,{Evidence:{Unknown:'value'}}).join()).toContain('value'); });
});
describe('асинхронное задание', () => {
  it('принимает начальное состояние, прогресс, завершение и причину сбоя', () => {
    const progress = {stage:'queued',completed:0,total:0};
    expect(analysisJob({id,status:'pending',progress})).toBe(true);
    expect(analysisJob({id,status:'running',progress:{stage:'analyzing',completed:2,total:4}})).toBe(true);
    expect(analysisJob({id,status:'completed',progress:{stage:'completed',completed:1,total:1},result:fixture.result})).toBe(true);
    expect(analysisJob({id,status:'failed',progress:{stage:'failed',completed:0,total:1},error:'provider unavailable'})).toBe(true);
  });
  it('отклоняет некорректный UUID и неполный прогресс', () => {
    expect(analysisJob({id:'invalid',status:'pending',progress:{stage:'queued',completed:0,total:0}})).toBe(false);
    expect(analysisJob({id,status:'running',progress:{stage:'analyzing',completed:-1,total:1}})).toBe(false);
  });
});
it('отклоняет повреждённые вложенные элементы', () => {
 const base = {id,result:{Plan:{Status:'parsed',Entries:null,Blockers:null,Diagnostics:null},Analysis:{Status:'parsed',Findings:null},Reports:null}};
 expect(envelope({...base,result:{...base.result,Plan:{...base.result.Plan,Entries:[null]}}})).toBe(false);
 expect(envelope({...base,result:{...base.result,Analysis:{Status:'parsed',Findings:[null]}}})).toBe(false);
 expect(envelope({...base,result:{...base.result,Reports:[null]}})).toBe(false);
});

it('сохраняет finding при неполном evidence', () => { expect(evidence({Entries:[]},{EntryIndex:0,Evidence:{Tokens:[null]}}).join()).toContain('недоступно'); expect(evidence({Entries:[{Installed:{Versions:[{Version:'1'}]}}]},{EntryIndex:0,Evidence:{InstalledVersionIndex:0}}).join()).toContain('1'); expect(evidence({}, {Evidence:{ContextIndex:null}}).join()).toContain('недоступно'); expect(evidence({Entries:[{SourceLines:[1]}]}, {EntryIndex:0,Evidence:{Unknown:'value'}}).join()).toContain('value'); });

it('принимает fixture сериализации Go и отклоняет повреждённые обязательные поля', () => {
 expect(envelope(fixture)).toBe(true);
 const broken = structuredClone(fixture); (broken.result.Plan.Entries[0] as any).Installed = null; expect(envelope(broken)).toBe(false);
 const optional = structuredClone(fixture); delete (optional.result.Plan.Entries[0] as any).FlagGroups; expect(envelope(optional)).toBe(true);
 expect(envelope({...fixture,result:{...fixture.result,Reports:undefined}})).toBe(false);
});
