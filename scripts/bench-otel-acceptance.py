#!/usr/bin/env python3
"""Compare existing rendered GoForj projects using identical HTTP acceptance fixtures."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import statistics
import subprocess
import tempfile
import time


def command(args, directory, environment):
    """command fails the comparison when compilation, correctness, or measured requests fail."""
    result = subprocess.run(args, cwd=directory, env=environment, capture_output=True, text=True, check=True)
    return result.stdout


def main():
    """main rotates base and candidate order while retaining raw output and every measured sample."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', type=Path, required=True)
    parser.add_argument('--candidate', type=Path, required=True)
    parser.add_argument('--base-ref', required=True)
    parser.add_argument('--candidate-ref', required=True)
    parser.add_argument('--runs', type=int, default=7)
    parser.add_argument('--benchtime', default='1s')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.runs < 1:
        parser.error('--runs must be positive')
    fixture = Path(__file__).resolve().parent.parent / 'internal/bench/testdata/otel_acceptance_bench_test.go'
    environment = os.environ.copy()
    environment.update(GOMAXPROCS='1', GOWORK='off')
    environment.setdefault('GOCACHE', '/tmp/gocache')
    environment.setdefault('GOMODCACHE', '/tmp/gomodcache')
    result = {'base_ref': args.base_ref, 'candidate_ref': args.candidate_ref,
              'go': command(['go', 'version'], args.base, environment).strip(),
              'host': platform.platform(), 'gomaxprocs': 1, 'runs': args.runs,
              'benchtime': args.benchtime, 'fixture_sha256': hashlib.sha256(fixture.read_bytes()).hexdigest(),
              'samples': [], 'raw': [], 'modules': {}}
    targets = {'base': args.base.resolve(), 'candidate': args.candidate.resolve()}
    with tempfile.TemporaryDirectory(prefix='goforj-otel-bench-') as binaries:
        for label, project in targets.items():
            target = project / 'internal/http/otel_acceptance_bench_test.go'
            if target.exists() and target.read_bytes() != fixture.read_bytes():
                raise RuntimeError(f'refusing to overwrite a different fixture: {target}')
            target.write_bytes(fixture.read_bytes())
            result['modules'][label] = command(['go', 'list', '-m', 'github.com/goforj/web'], project, environment).strip()
            command(['go', 'test', '-c', '-o', str(Path(binaries) / label), './internal/http'], project, environment)
        for sample in range(args.runs):
            for label in (['base', 'candidate'] if sample % 2 == 0 else ['candidate', 'base']):
                print(f'run {sample + 1}/{args.runs}: {label}', flush=True)
                output = command([str(Path(binaries) / label), '-test.run=^$', '-test.bench=^BenchmarkOTelAcceptance$',
                                  f'-test.benchtime={args.benchtime}', '-test.count=1'], targets[label], environment)
                result['raw'].append({'run': sample + 1, 'label': label, 'output': output})
                for line in output.splitlines():
                    match = re.match(r'^BenchmarkOTelAcceptance/(\w+)/(\w+)(?:-\d+)?\s+\d+\s+(.+)', line)
                    if match:
                        fields = match.group(3).split()
                        values = {fields[i + 1]: float(fields[i]) for i in range(0, len(fields), 2)}
                        result['samples'].append({'run': sample + 1, 'label': label,
                                                  'mode': match.group(1), 'workload': match.group(2), **values})
                args.output.parent.mkdir(parents=True, exist_ok=True)
                args.output.write_text(json.dumps(result, indent=2) + '\n')
    summary = []
    for mode, workload in sorted({(row['mode'], row['workload']) for row in result['samples']}):
        row = {'mode': mode, 'workload': workload}
        for label in targets:
            samples = [sample for sample in result['samples'] if (sample['label'], sample['mode'], sample['workload']) == (label, mode, workload)]
            if samples:
                if len(samples) != args.runs:
                    raise RuntimeError(f'incomplete samples for {label}/{mode}/{workload}')
                row[label] = {unit: statistics.median(sample[unit] for sample in samples) for unit in ['ns/op', 'B/op', 'allocs/op', 'requests/s', 'exported_pct'] if unit in samples[0]}
        if 'base' in row and 'candidate' in row:
            row['throughput_delta_percent'] = 100 * (row['candidate']['requests/s'] / row['base']['requests/s'] - 1)
        summary.append(row)
    result['summary'] = summary
    result['recorded_at_utc'] = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())
    args.output.write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(summary, indent=2))


if __name__ == '__main__':
    main()
