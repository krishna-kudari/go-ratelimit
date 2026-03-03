// bench/load_test.js — k6 load test for rate limit test server
// Run: k6 run -e ALGO=gcra bench/load_test.js
// Or loop: for algo in gcra fixed token cms prefilter; do LIMITER_MODE=$algo go run testserver/main.go & sleep 1; k6 run -e ALGO=$algo bench/load_test.js; kill %1; sleep 1; done
import http from 'k6/http';
import { check } from 'k6';
import { Rate } from 'k6/metrics';

const ALGO = (typeof __ENV !== 'undefined' && __ENV.ALGO) || 'gcra';
const rateLimited = new Rate('rate_limited');

export const options = {
  stages: [
    { duration: '10s', target: 50 },
    { duration: '30s', target: 500 },
    { duration: '10s', target: 1000 },
    { duration: '10s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<50', 'p(99)<100'],
    // No http_req_failed — 429 is correct behavior, not a failure
  },
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
};

export default function () {
  const keyId = Math.floor(Math.random() * 1000);
  const res = http.get('http://localhost:8080/api/test', {
    headers: { 'X-API-Key': `user-${keyId}` },
  });

  check(res, {
    'status 200 or 429': (r) => r.status === 200 || r.status === 429,
    'has ratelimit header': (r) => r.headers['X-Ratelimit-Limit'] !== undefined,
    '429 has retry-after': (r) => r.status !== 429 || r.headers['Retry-After'] !== undefined,
  });

  rateLimited.add(res.status === 429);
}

export function handleSummary(data) {
  return {
    [`bench/summary_${ALGO}.json`]: JSON.stringify(data, null, 2),
    stdout: formatSummary(data, ALGO),
  };
}

function formatSummary(data, algo) {
  const d = data.metrics.http_req_duration?.values ?? {};
  const r = data.metrics.http_reqs?.values ?? {};
  const rl = data.metrics.rate_limited?.values?.rate ?? 0;
  // k6 reports http_req_duration in ms — do not multiply by 1000
  const fmt = (v) => (v != null && typeof v === 'number' ? v.toFixed(2) : '-');

  return `
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Results (${algo})
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  req/sec      : ${fmt(r.rate)}
  total reqs   : ${r.count ?? 0}
  rate limited : ${fmt(rl * 100)}%

  latency (ms):
    p50 : ${fmt(d.med)}
    p95 : ${fmt(d['p(95)'])}
    p99 : ${fmt(d['p(99)'])}
    max : ${fmt(d.max)}
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
`;
}
