#!/usr/bin/env bash
set -euo pipefail

if [[ $# != 3 ]]; then
  printf 'Usage: %s BASE_BINARY CANDIDATE_BINARY NEW_OUTPUT_DIRECTORY\n' "$0" >&2
  exit 2
fi
baseline=$1
candidate=$2
output=$3
mkdir "$output"
: > "$output/results.jsonl"

for binary in "$baseline" "$candidate"; do
  [[ -x "$binary" ]] || { printf 'Not executable: %s\n' "$binary" >&2; exit 2; }
done

for vus in 1 100 256; do
  for block in 1 2 3; do
    position=0
    for variant in A B B A; do
      position=$((position + 1))
      binary=$baseline
      if [[ $variant == B ]]; then binary=$candidate; fi
      stem="$output/vus${vus}-block${block}-pos${position}-${variant}"
      GOMAXPROCS=4 GOGC=100 GOMEMLIMIT=off "$binary" run baseline -d noop \
        --steps workload --executor constant-vus --vus "$vus" --duration 8s \
        --report-format json --no-save-report > "$stem.json" 2> "$stem.stderr"
      jq -e '.status == "completed" and .errors.terminal_errors == 0 and
        .errors.failed_iterations == 0 and .errors.failed_queries == 0 and
        .errors.retry_attempts == 0 and
        .metrics.successful_transactions_total.total == .metrics.iterations_total.total' \
        "$stem.json" > /dev/null
      jq -c --arg variant "$variant" --argjson vus "$vus" --argjson block "$block" \
        '{variant:$variant,vus:$vus,block:$block,tps:.metrics.tps.total}' "$stem.json" \
        >> "$output/results.jsonl"
    done
  done
done

jq -s '
  def median: sort | .[length / 2 | floor];
  group_by([.vus,.block]) |
  map({vus:.[0].vus,block:.[0].block,
    baseline:([.[]|select(.variant=="A")|.tps]|add/length),
    candidate:([.[]|select(.variant=="B")|.tps]|add/length)}) |
  map(. + {ratio:(.candidate/.baseline)}) |
  group_by(.vus) |
  map({vus:.[0].vus,median_ratio:([.[].ratio]|median),
    minimum_ratio:([.[].ratio]|min),maximum_ratio:([.[].ratio]|max),blocks:.})
' "$output/results.jsonl" | tee "$output/summary.json"

jq -e 'all(.[]; if .vus == 1 then .median_ratio >= 0.95
  else .median_ratio >= 1.20 end)' "$output/summary.json" > /dev/null
