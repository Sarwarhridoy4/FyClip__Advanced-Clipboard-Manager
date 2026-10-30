# FyClip Performance Improvement Suggestions

This document proposes concrete performance optimizations for FyClip, organized by impact and effort. Each suggestion references the relevant source files and line numbers, and ties back to the existing benchmarks documented in `README.md`.

## Current Baseline (from README.md)

| Benchmark | ns/op | B/op | allocs/op | Status |
|---|---|---|---|---|
| `BenchmarkUpdateFilteredSearch1000` | ~104,000 | 255 | 5 | Implemented (sort.Slice) |
| `BenchmarkAddItemWithDuplicateScan1000` | ~430,000 | ~158,500 | ~1,400 | Implemented (sort.Slice + batch removal) |
| `BenchmarkStorageSave1000` | ~4,100,000 | ~540,000 | ~2,030 | Implemented (gzip pool + dirty flag) |

Measured with `go test -bench '.' -benchmem -count=3 ./internal/clipboard` on Linux/Pentium G4560.
The `AddItem` and `Storage.Save` benchmarks dominate cost and allocation. Implemented changes target those paths first.

---

## Implemented Changes

### 1. trimHistory — O(n²) bubble sort → sort.Slice + descending-index removal ✅

**File:** `internal/clipboard/manager.go` (`trimHistory`, ~line 392)

`trimHistory` previously sorted all unpinned items with a nested loop (O(n²)) and looked up each item's current index by scanning all history (O(n) per removal). Now uses `sort.Slice` by `LastAccessed` ascending, collects removal indices, sorts them, and removes from highest to lowest so earlier removals don't shift later indices.

**Impact:** `BenchmarkAddItemWithDuplicateScan1000` improved ~5.3x (2.15ms → ~0.43ms).

### 2. updateFiltered — O(n²) bubble sort → sort.Slice ✅

**File:** `internal/clipboard/manager.go` (`updateFiltered`, ~line 493)

When `sortByDate` is enabled, unpinned items are now sorted with `sort.Slice` instead of a bubble sort. Sorts descending (newest first) to match the original behavior and existing tests.

**Impact:** O(n log n) date sort; faster UI refresh on search/filter changes with date sorting enabled.

### 3. gzip buffer pooling ✅

**File:** `internal/clipboard/storage.go` (`compress`, `decompress`)

Added `gzipWriterPool` and `gzipReaderPool` (`sync.Pool`) to reuse `*gzip.Writer`/`*gzip.Reader` across save/load cycles instead of allocating new ones each call. Writers are reset and returned to the pool after use.

**Impact:** `BenchmarkStorageSave1000` allocation dropped from 1,292,749 B/op to ~540,000 B/op (~59% less).

### 4. Dirty-flag save skip ✅

**File:** `internal/clipboard/manager.go`

Added a `dirty bool` field on `Manager`. Set to `true` in `AddItem`, `removeAtIndex`, and `ClearUnpinned`. `saveHistoryNow` now returns early when `dirty` is false, skipping the marshal/gzip/AES-encrypt/write cycle when nothing changed since the last save. The flag is reset to `false` after a successful save.

**Impact:** `saveHistoryNow` becomes a no-op during idle periods with no clipboard changes.

### 5. Regex cache LRU eviction ✅

**File:** `internal/clipboard/search.go`

Added `regexOrder []string` tracking insertion order. When the cache reaches 128 entries, the oldest entry is evicted (`evictOldestRegexLocked`) instead of discarding the entire map with `regexCache = make(...)`.

**Impact:** No more cache-thrash recompilation spikes when many distinct patterns are used.

### 6. memoryPressureCheck frequency ✅

**File:** `internal/clipboard/manager.go` (`CheckMemoryPressure`)

Base interval increased 30s → 60s; under pressure interval 10s → 20s. Reduces `runtime.ReadMemStats` calls (which force a GC scan) for long-running sessions.

### 7. FilteredSeq range-over-func ✅

**File:** `internal/clipboard/manager.go` (`FilteredSeq`)

Added `FilteredSeq(yield func(Item) bool)` alongside the existing `GetFiltered()`. Iterates filtered items under `RLock` without allocating a copy of the slice. Callers that only need to read can use `FilteredSeq` to avoid the `[]Item` copy.

---

## Remaining / Not Implemented

### Thumbnail skip-if-exists (already present) ✅

`Item.EnsureThumbnail()` (`item.go:184-188`) already skips regeneration when `Thumbnail != ""`. Thumbnails are persisted in the JSON serialization, so they survive restarts. No change needed.

### os.Stat in validateFilePath (intentionally kept)

`validateFilePath` (`manager.go`) retains its `os.Stat` safety check. Files can be deleted between clipboard capture and paste/preview, so re-validation is a deliberate defense-in-depth measure, not a performance bug.

### bytes.Builder/strings.Builder audit (already compliant)

`stripHTMLForSearch`, `sanitizeDisplayText`, `escapeHTML`, and `escapeMarkdown` all already use `strings.Builder`. No remaining `+=` string concatenation in hot paths.

### Defer non-critical saves until idle (not implemented)

The debounced save dispatcher (`manager.go`, ~line 1493) already coalesces saves with a 250ms debounce. Aggressive idle-based deferral (1–2s) was not implemented because it risks losing recent clipboard changes on unexpected exit.

### pprof confirmation

Baseline profiling commands from the original suggestions remain valid:

```bash
go test -bench . -benchmem ./internal/clipboard
go test -cpuprofile cpu.prof -memprofile mem.prof -bench . ./internal/clipboard
go tool pprof cpu.prof
```

### iter.Seq (not implemented — Go version constraint)

`iter.Seq` requires Go 1.23+. `GetFiltered` returns `[]Item` for backward compatibility with existing callers. `FilteredSeq` (implemented above) provides the zero-copy iteration alternative without requiring `iter`.

---

## Suggested Priority Order (status)

1. ✅ `trimHistory` sort.Slice + batch removal — biggest win for `AddItem` under load
2. ✅ `updateFiltered` sort.Slice — easy win for date-sorted UI
3. ✅ Thumbnail skip-if-exists — already present in `EnsureThumbnail`
4. ✅ Dirty-flag save skip — biggest win for `Storage.Save`
5. ✅ gzip buffer pool — lower alloc rate
6. ✅ Regex cache LRU — stable search latency
7. ✅ memoryPressureCheck interval — lower background CPU overhead
8. ✅ FilteredSeq — zero-copy filtered iteration

---

## Files Modified

- `internal/clipboard/manager.go` — `AddItem` (dirty flag), `trimHistory` (sort.Slice), `updateFiltered` (sort.Slice), `removeAtIndex` (dirty flag), `ClearUnpinned` (dirty flag), `saveHistoryNow` (dirty skip), `CheckMemoryPressure` (intervals), `FilteredSeq` (new)
- `internal/clipboard/storage.go` — `compress` (gzip pool), `decompress` (gzip pool), pool declarations
- `internal/clipboard/search.go` — regex cache LRU with `regexOrder`
