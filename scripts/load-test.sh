#!/bin/sh
set -eu
mkdir -p output-tdd/performance
k6_output=output-tdd/performance/k6-contract.txt
if ! docker run --rm --network host -i grafana/k6:1.3.0 run - <load/k6-contract.js >"$k6_output" 2>&1; then
	cat "$k6_output" >&2
	exit 1
fi
cat "$k6_output"

benchmark_output=output-tdd/performance/go-benchmarks.txt
if ! go test -run='^$' -bench=. -benchmem ./internal/matching ./internal/position ./internal/margin ./internal/liquidation >"$benchmark_output" 2>&1; then
	cat "$benchmark_output" >&2
	exit 1
fi
cat "$benchmark_output"
uname -a > output-tdd/performance/machine.txt
