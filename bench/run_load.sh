#!/usr/bin/env bash
# Run k6 load test for each algorithm. From repo root: ./bench/run_load.sh
# Ensure nothing is listening on 8080 before running (or run: lsof -ti:8080 | xargs kill -9)
set -e
cd "$(dirname "$0")/.."

kill_port() {
  lsof -ti:8080 | xargs kill -9 2>/dev/null || true
  sleep 1
}

for algo in gcra fixed token cms prefilter; do
  echo "── $algo ──"
  kill_port
  LIMITER_MODE=$algo go run testserver/main.go &
  PID=$!
  sleep 2
  k6 run -e ALGO=$algo bench/load_test.js || true
  kill $PID 2>/dev/null || true
  kill_port
done

echo ""
echo "Summary:"
python3 bench/parse_summaries.py
