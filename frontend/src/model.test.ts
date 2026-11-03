import { describe, expect, it } from 'vitest';
import { envelope, evidence, input } from './model';
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
it('отклоняет повреждённые вложенные элементы', () => {
 const base = {id,result:{Plan:{Status:'parsed',Entries:null,Blockers:null,Diagnostics:null},Analysis:{Status:'parsed',Findings:null},Reports:null}};
 expect(envelope({...base,result:{...base.result,Plan:{...base.result.Plan,Entries:[null]}}})).toBe(false);
 expect(envelope({...base,result:{...base.result,Analysis:{Status:'parsed',Findings:[null]}}})).toBe(false);
 expect(envelope({...base,result:{...base.result,Reports:[null]}})).toBe(false);
});

it('сохраняет finding при неполном evidence', () => { expect(evidence({Entries:[]},{EntryIndex:0,Evidence:{Tokens:[null]}}).join()).toContain('недоступно'); expect(evidence({Entries:[{Installed:{Versions:[{Version:'1'}]}}]},{EntryIndex:0,Evidence:{InstalledVersionIndex:0}}).join()).toContain('1'); expect(evidence({}, {Evidence:{ContextIndex:null}}).join()).toContain('недоступно'); expect(evidence({Entries:[{SourceLines:[1]}]}, {EntryIndex:0,Evidence:{Unknown:'value'}}).join()).toContain('value'); });
