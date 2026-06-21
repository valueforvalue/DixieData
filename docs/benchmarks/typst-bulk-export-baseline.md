# Typst bulk-export baseline

Issue: [#66](https://github.com/valueforvalue/DixieData/issues/66).

Captures the wall-clock cost of the current `ExportFullDatabasePDF` typst path on the per-record loop. Run before any optimization work in #67 so we have something concrete to compare against.

## How to reproduce

```sh
go test ./internal/archive/ -run '^TestFullDatabasePDFBaseline$' -v -count=1
```

Output is JSON written to `build/log/bulk-bench-<timestamp>.json` (override with `BULK_BENCH_OUT=...`).

The benchmark seeds N records (each with a 1×1 PNG portrait and a ~20-sentence biography), runs `ExportFullDatabasePDF`, and reports:

- `seed_ms` — time to insert N rows + images
- `export_ms` — pure export wall-clock
- `ms_per_record` — `export_ms / N`
- `pdf_size_bytes` — size of the PDF at the user-chosen path (currently `-1`; the typst path writes a folder of per-record PDFs instead — see #64)
- `record_dir_record_count` — count of per-record PDFs in the sibling directory
- `record_dir_total_bytes` — total size of the per-record PDFs

## Captured baseline (commit `473b5d6`, 2026-06-21)

Fixture: 100 soldiers, 100 images, each soldier ~400 words of biography, single `typst compile` per soldier.

| Metric | Value |
| --- | ---: |
| `seed_ms` | 155 |
| `export_ms` | **45 766** |
| `ms_per_record` | **457.66** |
| `record_dir_record_count` | 100 |
| `record_dir_total_bytes` | 8 694 235 |
| `sample_record_bytes` | 86 126 |
| `pdf_size_bytes` | -1 (no single PDF at chosen path — #64 regression) |

Host: Windows 11, AMD64, `bin/typst-windows.exe` 0.15.0, Go 1.23.

## Extrapolation

Linear in N (no caching, no shared state between records):

| Records | Predicted `export_ms` | Predicted wall-clock |
| ---: | ---: | --- |
| 100 | 45 766 | 46 s |
| 500 | 228 830 | ~3 min 49 s |
| 1 000 | 457 660 | ~7 min 38 s |
| 3 000 | 1 372 980 | ~22 min 53 s |

User-reported archive (500 records growing to 3 000+) matches this band. The 23-minute figure for 3 000 records is consistent with the user's "very long time" report.

## Where the time goes

Each record goes through `pkg/render/renderers.go::TypstRenderer.Render`:

1. `os.MkdirTemp` — fresh tempdir
2. `copyDir(template_tree)` — recursive copy of `templates/` (~10 files in `common/`) into the tempdir
3. `copyFile(tpl.Path → main.typ)` — copy the chosen template file
4. `stageSoldierImages` — `copyFile` per image
5. `writeJSONFile(data.json)` — serialize the soldier record
6. `exec.Command(typst compile)` — fresh child process, cold-starts the 30 MB typst binary
7. `os.Open(out.pdf)` + `io.Copy(w, f)` — stream the PDF back

Steps 1, 2, and 6 dominate on Windows. Each `typst compile` cold-start costs ~50-100 ms on this hardware; the per-record template-tree copy and image copy add another ~50-150 ms depending on image size.

## Implications for #67

If slice A (#64) lands and the export becomes a single typst invocation over an array of records, the per-record cold-start cost disappears. The remaining cost is one typst compile for the whole archive plus the N image copies (which can be parallelized and pre-staged). Target for #67:

- **500 records: under 30 s**
- **3 000 records: under 3 min**

If slice A is in effect by the time #67 runs, the 500/3000 target is realistically achievable without further code changes beyond what slice A already provides.

## Out of scope for this baseline

- Per-renderer instrumentation of `copyDir` / `stageSoldierImages` / `runTypstCompile`. Add it in #67 if the post-#A numbers still fall short.
- `pprof` CPU/memory profiles. Add if a single typst invocation still takes >30 s.
- Fpdf path baseline. Retired in slice 7 (commit `39a4909`); no comparison available.