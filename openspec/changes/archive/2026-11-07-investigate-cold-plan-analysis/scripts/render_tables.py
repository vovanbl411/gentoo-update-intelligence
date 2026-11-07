"""Render measured values only; interpretation stays in the authored report."""
import argparse
import collections
import json
from pathlib import Path
import statistics


def distribution(values, scale=1):
    values = [v / scale for v in values]
    return f'{statistics.median(values):.2f} [{min(values):.2f}–{max(values):.2f}]'


def table(headers, rows):
    return '\n'.join(['| ' + ' | '.join(headers) + ' |', '| ' + ' | '.join(['---'] * len(headers)) + ' |'] + ['| ' + ' | '.join(map(str, row)) + ' |' for row in rows])


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('summary', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    data = json.loads(args.summary.read_text())
    runs = [r for r in data['runs'] if r['successful']]
    out = ['# Таблицы измерений', '', 'Все времена — секунды. Время HTTP — от начала POST до первого наблюдения. Интервалы в квадратных скобках обозначают минимум и максимум.', '']
    out += [table(['Прогон', 'Первый partial', 'Первый metadata', 'Терминальное состояние', 'Generated-cache hits', 'Генерации'], [[r['label'], f"{r['first_partial_s']:.3f}", f"{r['first_metadata_s']:.3f}", f"{r['terminal_s']:.3f}", r['worker']['cache_hits'], r['worker']['generated']] for r in runs]), '']
    out += [table(['Прогон', 'CPU user/system, с', 'Память cgroups, GiB', 'Anon/file, GiB', 'vda read/write, GiB', 'Мин. MemAvailable, MiB', 'Мин. место, GiB'], [[r['label'], f"{r['metrics']['cpu_user_s']:.2f}/{r['metrics']['cpu_system_s']:.2f}", f"{r['metrics']['sampled_memory_peak_bytes']/2**30:.3f}", f"{r['metrics']['sampled_anon_peak_bytes']/2**30:.3f}/{r['metrics']['sampled_file_peak_bytes']/2**30:.3f}", f"{r['metrics']['device_read_bytes']/2**30:.3f}/{r['metrics']['device_write_bytes']/2**30:.3f}", f"{r['metrics']['guest_min_available_bytes']/2**20:.1f}", f"{r['metrics']['min_free_bytes']/2**30:.2f}"] for r in runs]), '']
    out += ['## Непрерывность наблюдений', '', table(['Прогон', 'Сэмплы', 'Макс. gap guest wall, с', 'Макс. gap guest monotonic, с', 'Макс. gap host wall, с'], [[r['label'], r.get('sampling', {}).get('samples', 0), f"{r.get('sampling', {}).get('guest_wall_gap_max_s', 0):.3f}", f"{r.get('sampling', {}).get('guest_monotonic_gap_max_s', 0):.3f}", f"{r.get('sampling', {}).get('host_wall_gap_max_s', 0):.3f}"] for r in data['runs']]), '']
    groups = collections.defaultdict(list)
    for r in runs:
        groups[f"plan{r['plan']}-" + ('control-' if '-control-' in r['label'] else 'instrumented-') + r['label'].rsplit('-', 1)[1]].append(r)
    out += ['## Медианы и диапазоны', '', table(['Группа', 'n', 'Wall, с', 'CPU user, с', 'CPU system, с', 'Память, GiB', 'Запись vda, GiB'], [[key, len(group), distribution([r['terminal_s'] for r in group]), distribution([r['metrics']['cpu_user_s'] for r in group]), distribution([r['metrics']['cpu_system_s'] for r in group]), distribution([r['metrics']['sampled_memory_peak_bytes'] for r in group], 2**30), distribution([r['metrics']['device_write_bytes'] for r in group], 2**30)] for key, group in sorted(groups.items())]), '']
    out += ['## Этапы worker в холодных прогонах', '', 'Листовые этапы при одном preparation slot выполняются последовательно. `worker.prepare` включает вложенные этапы; ожидание preparation slot перекрывается с активной подготовкой и не прибавляется к wall time.', '']
    leaves = ['worker.checkout.main', 'worker.checkout.master', 'worker.cleanup', 'worker.generator_setup', 'worker.egencache', 'worker.portage_version', 'worker.resolver', 'provider.repository_cache', 'worker.generated_cache.load', 'worker.generated_cache.save', 'worker.profile', 'worker.generation_slot_wait']
    rows = []
    for stage in leaves + ['worker.prepare', 'worker.preparation_slot_wait']:
        row = [stage]
        for plan in [2, 3]:
            selected = groups[f'plan{plan}-instrumented-cold']
            row.append(distribution([r['worker']['seconds'].get(stage, 0) for r in selected]))
        rows.append(row)
    out += [table(['Этап', 'План 2, с', 'План 3, с'], rows), '']
    out += ['## Компоненты в холодных прогонах', '']
    rows = []
    for plan in [2, 3]:
        selected = groups[f'plan{plan}-instrumented-cold']
        for component in ['cold-backend', 'cold-worker', 'cold-postgres']:
            components = [r['components'][component] for r in selected]
            rows.append([plan, component, distribution([r['cpu_user_s'] for r in components]), distribution([r['cpu_system_s'] for r in components]), distribution([r['sampled_anon_peak_bytes'] for r in components], 2**20), distribution([r['sampled_file_peak_bytes'] for r in components], 2**20), distribution([r['cgroup_write_bytes'] for r in components], 2**30)])
    out += [table(['План', 'Компонент', 'CPU user, с', 'CPU system, с', 'Anon, MiB', 'File, MiB', 'Запись cgroup, GiB'], rows), '']
    out += ['## Наблюдатель и помехи хоста', '', table(['Прогон', 'CPU наблюдателя user/system, с', 'RSS наблюдателя, MiB', 'Запись наблюдателя, MiB', 'Host swap in/out, MiB', 'Host load1 max', 'Guest iowait/steal, с'], [[r['label'], f"{r['metrics']['observer_cpu_user_s']:.2f}/{r['metrics']['observer_cpu_system_s']:.2f}", f"{r['metrics']['observer_sampled_rss_peak_bytes']/2**20:.2f}", f"{r['metrics']['observer_write_bytes']/2**20:.2f}", f"{r['metrics'].get('host_swap_in_pages',0)*4096/2**20:.2f}/{r['metrics'].get('host_swap_out_pages',0)*4096/2**20:.2f}", f"{r['metrics'].get('host_load1_max',0):.2f}", f"{r['metrics']['guest_iowait_s']:.2f}/{r['metrics']['guest_steal_s']:.2f}"] for r in runs]), '']
    args.output.write_text('\n'.join(out))
