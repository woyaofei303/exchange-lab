#!/bin/sh
set -eu
go test -run='^$' -fuzz=FuzzDecodeNeverPanics -fuzztime=3s ./internal/marketdata
go test -run='^$' -fuzz=FuzzEngineNeverOverfills -fuzztime=3s ./internal/matching
go test -run='^$' -fuzz=FuzzApplyFillReplay -fuzztime=3s ./internal/position
