#!/usr/bin/env python3
"""Parse bench/summary_*.json from k6 runs into a README-style table."""
import json
import glob
import os

def main():
    files = sorted(glob.glob(os.path.join(os.path.dirname(__file__), 'summary_*.json')))
    if not files:
        print("No bench/summary_*.json files found. Run: k6 run -e ALGO=gcra bench/load_test.js")
        return

    print(f"{'Algorithm':<15} {'req/sec':>10} {'p95':>10} {'p99':>10} {'rate_limited':>14}")
    print("-" * 65)

    for path in files:
        algo = os.path.basename(path).replace('summary_', '').replace('.json', '')
        with open(path) as fp:
            data = json.load(fp)

        m = data.get('metrics', {})
        rps = m.get('http_reqs', {}).get('values', {}).get('rate', 0)
        dur = m.get('http_req_duration', {}).get('values', {})
        p95 = dur.get('p(95)', 0)
        p99 = dur.get('p(99)', 0)
        rl = m.get('rate_limited', {}).get('values', {}).get('rate', 0) * 100

        print(f"{algo:<15} {rps:>10.0f} {p95:>9.2f}ms {p99:>9.2f}ms {rl:>13.2f}%")


if __name__ == '__main__':
    main()
