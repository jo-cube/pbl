# Ingestion performance notes

Measured 2026-10-26 on an Apple M4 MacBook Air, 24 GB RAM, macOS 26.5.1,
Go 1.26.5, Pebble v2.1.6, and local APFS storage. These are warm synthetic
workloads on one unisolated host, not production Kafka throughput guarantees.

## Retained design

Bulk ingestion uses ordinary Pebble batches and the existing ordered decoders:

- 16 MiB steady-state memtables reduce flush frequency. Pebble's small initial
  allocation remains; larger/recycled WAL files and queued memtables cost space.
- A fixed 128-key sample enables coalescing when at least 16 sampled operations
  repeat keys. A stable sort retains the last put/delete per full physical key
  within the atomic batch. Input operation counts and byte limits still determine
  commits. Scratch buffers are cleared and pooled only up to 4,096 entries.
- Successful streaming import, apply, and del-many commands flush outstanding
  WAL tails above 4 MiB before close, bounding next-command replay. Smaller tails
  avoid this extra IO. Failure paths retain earlier committed batches and discard
  pending operations. Final flush errors propagate after committed data remains.
- The optional application delete Bloom filter uses eleven bits per expected key,
  rounded to 64-byte blocks, one seeded 64-bit hash, six local multiplicative
  probes, and a fixed one-bit reduction. It allocates no memory per operation.

The filter covers initially live keys and every incoming put, including pending
puts. It never clears bits. Repeated puts need no additional capacity; deletes
of formerly live keys remain positive until a subsequent invocation rebuilds
from recovered live state. Underestimation raises false positives, not false
negatives. No per-delete database lookup or persistent filter state is added.

Storage format v1, metadata, opaque values, CLI flags/output/exit codes, per-batch
sync policy, and input ordering remain unchanged. Pebble's separate 10-bit SST
filters, Snappy compression, and compaction settings remain unchanged. Coalescing
is valid because this batch wrapper only accepts puts and point deletes, and
intermediate operations within an atomic batch are not separately visible.

## Ingestion measurements

Baseline: `f876ec5`, before the memtable/coalescing/tail-flush changes. This
comparison retains the original application Bloom filter on both sides. The
separate Bloom results below start after these ingestion changes; do not multiply
speedup ratios from the two baselines.

CLI medians of three runs, each using `-benchtime=3x`. Generated input is outside
the timer; DB initialization, parsing, writes, normal tail flushing, and close
are included. Values are 256 compressible bytes with a changing version prefix.

| Workload | Before ms | After ms | Allocated MB, before → after |
| --- | ---: | ---: | --- |
| 500K unique shuffled keys | 1562.7 | 618.9 | 30.22 → 14.81 |
| 500K churn over 50K keys | 900.3 | 400.8 | 13.81 → 6.67 |
| 500K churn over 256 hot keys | 747.1 | 266.3 | 6.87 → 2.16 |
| 1M noisy records, filter off | 676.4 | 323.6 | 8.59 → 5.67 |
| 1M noisy records, original filter on | 110.0 | 109.0 | 1.73 → 1.35 |

A larger direct-store experiment used two million shuffled unique keys, 256-byte
high-entropy values sampled from a seeded pool, and 1,000-operation batches.
Medians of three independent runs include explicit flush/full compaction/close:

| Metric | Before | After |
| --- | ---: | ---: |
| Producer time, s | 21.16 | 9.01 |
| Complete time, s | 27.41 | 16.65 |
| Process CPU, s | 13.87 | 9.05 |
| Go allocated MB | 259.09 | 156.97 |
| SST output GB | 4.044 | 2.795 |
| Logical WAL MB | 584.02 | 584.02 |
| Final live SST MB | 549.19 | 549.19 |
| Closed directory file lengths, MB | 563.36 | 607.71 |
| Peak sampled memtable allocation, MB | 12.32 | 50.07 |
| Peak sampled compaction debt, MB | 493.86 | 588.92 |

Larger memtables reduce total table writes but can leave more compaction work
at the producer boundary. Live stored data is unchanged; recycled WAL space
explains most of the larger closed directory. With small live states the
relative cost is larger: one broad-churn case occupied about 17.7 → 60.7 MB
in file lengths despite only about 3.8 MB of live SST data.

The final-tail flush belongs with the larger memtable change. After 500K hot-key
records, first-scan medians were 40.8 ms before, 54.7 ms with larger memtables and
coalescing alone, and 26.7 ms with the retained tail flush. The flush adds about
19 ms to that candidate's ingestion; total workflow time was too variable to
claim a separate statistically established improvement. Warm settled read checks
found no material regression, without establishing cold/concurrent-read latency.

## Bloom measurements

Baseline: the previous 10-bit, two-hash, seven-probe application filter, with the
retained ingestion changes present on both sides. The replacement is the retained
11-bit, six-probe blocked filter described above.

The full-size bounded Kcat stream contains two billion records, ten million
distinct puts, 1.99 billion never-added deletes, 24-byte keys, and 256-byte
compressible values. Put IDs are sequential, absent deletes randomized, and the
input totals 60.57 GB. Timing includes open, allocation/preload, input generation,
normal apply, explicit final flush/full compaction, and close. This harness calls
the normal apply path directly, excluding Cobra setup and stats formatting.

| Metric | Original filter | Blocked filter |
| --- | --- | --- |
| Complete trials, s | 152.184, 149.980, 149.392 | 122.663, 121.482 |
| Mean complete time, s | 150.519 | 122.073 |
| Mean process CPU, s | 153.283 | 124.752 |
| Mean false-positive deletes | 2,672,848 | 2,383,219 |
| Mean WAL output MB | 3,024.856 | 3,014.708 |
| Mean SST output MB | 556.792 | 550.629 |
| Mean final SST MB | 241.710 | 241.704 |
| Bitset bytes | 12,500,000 | 13,750,016 |

This is 23.3% higher throughput and 18.6% less CPU, chiefly a lookup-cost gain.
The growing filter's lifetime false-positive rate is 0.1343% → 0.1198%; it is
not the saturated rate. Final counts/digests match and reported compaction debt
is zero. Fresh-process scans and mixed get-many checks found no measurable
read regression. The bitset costs 10% more memory; peak process RSS ranges were
114.9–119.6 MB original and 118.5–120.0 MB blocked, including setup.

Three independently seeded filters per load, with ten million actual keys and
20 million absent queries each, illustrate the accuracy trade-off:

| Load | Original false positives | Blocked false positives |
| --- | ---: | ---: |
| Nominal capacity | 0.82002% | 0.64845% |
| Expected count = 2 × actual keys (half full) | 0.01946% | 0.02795% |

At nominal capacity, original/blocked occupancy is 50.34%/41.89%; the standard
deviation across 64-byte regions is 2.21/4.75 percentage points. Local probes
increase variation between blocks. Observed blocked false positives closely
match the mean of individual block occupancies raised to the probe count,
explaining why lower global occupancy need not improve underfilled accuracy.
No defective hash mixing was found. Ten-million-key construction measured
153.75 → 81.03 ms and absent lookup 33.83 → 8.70 ns, including query bookkeeping.
Both implementations already had zero per-operation allocations.

## Mixed-workload regressions

Twenty-million-record tests preload one million keys and include 100,000 new
puts. Historical targets are keys added previously, including already-deleted
keys. Main reproduction uses five alternating trials; other rows use three.
Complete times include final settling, and SST numbers include rewritten tables.

| Workload | Complete s, original → blocked | SST output MB, original → blocked |
| --- | --- | --- |
| 20M records, 1% historical targets | 2.274 → 1.926 | 74.882 → 93.411 |
| 20M records, 100% historical targets | 10.349 → 10.295 | 250.606 → 245.313 |
| 200M records, 1% historical, randomized puts | 16.972 → 13.014 | 649.429 → 648.324 |
| 20M records, 128-byte keys, never-added deletes | 2.698 → 2.000 | 8.003 → 32.677 |

The 1% write regression reproduces despite better filter accuracy. A frozen
capture passes 328,370 → 304,982 deletes, including 129,151 → 105,763 false
positives. Native mutation counts equal accepted counts after coalescing.
An event trace shows a 491,145 → 501,121-byte low-key tombstone table overlapping
about 19.8 MB below it. The ratio changes from 40.27 to 39.47, crossing the pinned
[Pebble density picker's](https://github.com/cockroachdb/pebble/blob/v2.1.6/compaction_picker.go)
40x exclusion boundary and triggering larger rewrites. Disabling density picking
only in a diagnostic overlay removes that increase (70.66 → 70.09 MB); it is not
retained because it changes engine policy and may leave tombstones that hurt reads.

Identical accepted-stream replay with identical batches/WAL but different read
rates produces 66.94–82.79 MB of SST output. Compaction timing and table boundaries
matter independently of filter accuracy. Randomized put order reverses the small
mixed-case write result (137.55 → 102.26 MB). In the longer randomized case above,
SST ranges overlap broadly (614.77–649.47 versus 613.61–648.34 MB); treat writes
as tied, rather than claiming the tiny median difference as a storage gain.

The earlier all-historical timing penalty did not reproduce in alternating or
matched-seed trials. Both filters pass exactly 19.9 million deletes and produce
identical 726,503,540 WAL bytes. Of these deletes, 18,805,409 target keys already
deleted earlier in the invocation. A monotonic filter must keep passing them.
The unfiltered path is preferable here: three matched trials measured
9.788 s without filtering versus 10.334 s with the current filter.

The long-key regression remains real. Events show the original draining new
tombstone tables and then making 28 trivial moves, while blocked ingestion
rewrites 29 L0 tables including the seed. Slowing blocked input still produces
32.69 MB of writes. Disabling density picking raises the original to about
33.15 MB instead of fixing blocked output. Exact per-block density at each pick
was not collected, so the mixed case's specific 40x cutoff is not established
for this case. Precompacting the seed removes the large difference in a separate
check, but is not a recommended extra step before each import.

## Alternatives and remaining limits

Unconditional sorting, larger default application batches, extra compaction
workers, compression changes, and small-file SST ingestion did not justify
retention. Presorting added allocation cost; 1K SST chunks produced excessive
L0 files. Sampled coalescing helps hot keys without paying that cost universally.
Eight MiB memtables remain a possible lower-footprint compromise, but were
slower in the larger ingestion experiment; 32 MiB added memory without a useful
broad-churn gain.

Seven blocked probes close the half-full accuracy gap but add lookup/build work.
Twelve bits/six probes reduce false positives and writes for another 9.1% bitset
memory. Three 200M-record trials over ten million existing keys measured about
17.7 s for 11/6, 11/7, and 12/6, versus 22.2 s original. There was no established
sustained-throughput benefit sufficient to change the retained 11/6 budget.
Additional hash/mixer, split-block, and fingerprint-set experiments offered no
compelling overall benefit. No alternate filter implementation or tuning flag
is retained.

The tests cover ordered reference-map state, duplicate keys, deletes, binary
values/keys, borrowed buffers, snapshots, batch failure boundaries, overflow,
underestimated capacity, and rebuilding from synced WAL recovery. Storage v1
reopen compatibility was checked in both directions. None of these results
establish exhaustive device fault/power-loss behavior, physical SSD write
amplification, cold reads, or multi-hour compaction behavior.

The next useful work is actual Kafka-trace replay on the target Linux CPU and
storage: measure distinct put cardinality, never-added versus historical deletes,
backpressure, realistic payload entropy, total CPU/RSS, WAL/SST output, long-term
compaction, and read latency. Avoid tuning filter constants or disabling density
compactions around one synthetic table threshold.

## Routine reproduction

```sh
go test ./tests/perf -run '^$' -bench BenchmarkKafkaApply -benchmem -benchtime=3x -count=3
go test ./internal/app -run '^$' -bench 'BenchmarkDeleteBloom(Build|Cardinality)$' -benchmem -benchtime=300ms -count=3
go test ./tests/perf -run '^$' -bench BenchmarkBloomApplyExisting -benchmem -benchtime=1x -count=3
```

The first benchmark uses shuffled unique/churn/hot keys and a one-million-record
99.5%-delete stream. The existing-DB benchmark uses bounded generation inside
the timer, four million records, 16-byte values, 50K/1M initial keys, and
0%/1%/50%/100% historical targets. Both measure real CLI completion, including
normal tail flush and close; they do not force final full compaction. Filter
lookup/build checks isolate cardinality and hit/miss costs.

For the ingestion baseline, use a separate checkout of `f876ec5` and copy only
`tests/perf/kafka_test.go` before running the first command. The routine benchmarks
cover workload families, not the exact two-billion-record or instrumented
compaction experiments. Full-run source snapshots, configurations, raw logs,
and traces are preserved in the investigation's external review bundles;
experimental harnesses and generated databases are not repository contents.
