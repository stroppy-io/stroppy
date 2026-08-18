# Golden SHA-256 hashes — TPC-B SF=1 via the CSV driver

Each `<table>.csv.sha256` is the hex-encoded SHA-256 of the
corresponding merged CSV emitted by `tpcb/tx` at a scale factor of 1 with the CSV driver's default options (`?merge=true`,
comma separator, headers on).

Shape: header row + 1 / 10 / 100_000 data rows for
`pgbench_branches` / `pgbench_tellers` / `pgbench_accounts`.

Hashes are computed over the full file (including the header), LF
line endings, RFC-4180 quoting as produced by `encoding/csv`.

## Regenerate

```
./build/stroppy run tpcb/tx \
  -D url='/tmp/tpcb-csv?merge=true&workload=tpcb_sf1' \
  -D driverType=csv \
  --scale-factor 1 \
  --load-workers 1 \
  --steps drop_schema,create_schema,load_data

sha256sum /tmp/tpcb-csv/tpcb_sf1/*.csv > new-hashes.txt
```

The CSV driver's merge pass concatenates worker shards in ascending
`w%03d.csv` order, so hashes are stable across worker counts.
