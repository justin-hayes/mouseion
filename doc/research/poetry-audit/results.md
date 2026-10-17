# Reproduction and observed results

Audit target: `6e02d180b1015536127839e0a45953684c0e5a83`, Linux, 2026-10-17. Application code is unchanged. These are investigation assets, not proposed permanent regression tests or CI changes.

## Environment and commands

Go **1.27.2**, Python **3.13.5**, uv **0.13.0**, Stanza **1.14.0**, grpcio **1.71.2**. Python environment was installed from this commit's `nlp/uv.lock`, binary-only, into task scratch; no model download or pipeline warmup. Docker was available for reusable PostgreSQL test infrastructure. An existing Python 3.11 environment was inspected but **not** used for the recorded Python runs.

From the repository/asset checkout (Go and uv on `PATH`):

```sh
export GOTMPDIR="$HOME/.cache/gotmp"
export AUDIT_SCRATCH="$HOME/.cache/scratch/poetry-audit"
mkdir -p "$GOTMPDIR" "$AUDIT_SCRATCH/tmp" "$AUDIT_SCRATCH/pycache"
export TMPDIR="$AUDIT_SCRATCH/tmp"
export UV_PROJECT_ENVIRONMENT="$AUDIT_SCRATCH/venv"
uv sync --project nlp --python 3.13 --locked --no-build --group dev
export PYTHONPATH=nlp/src:gen/python
export PYTHONPYCACHEPREFIX="$AUDIT_SCRATCH/pycache"

python3 doc/research/poetry-audit/run.py
"$UV_PROJECT_ENVIRONMENT/bin/python" doc/research/poetry-audit/producer_probe.py

go test -count=1 ./internal/epub ./internal/analyzer ./internal/analysis ./internal/selection ./internal/gdex ./internal/cardexport ./internal/enrichment ./internal/prepareddeck ./internal/persistence ./internal/webapp ./internal/opds
go test -count=1 -tags=integration ./internal/analysis ./internal/epub ./internal/persistence ./internal/prepareddeck ./internal/webapp
"$UV_PROJECT_ENVIRONMENT/bin/python" -m pytest -q -rs -p no:cacheprovider nlp/tests
make lint VENV="$UV_PROJECT_ENVIRONMENT"
```

The first targeted Go unit run used `GOTMPDIR` but did not override `TMPDIR`; subsequent probes, integration and Python commands did. `make lint` selects its own repository-local Go temp directory. The overlay runner creates and cleans its own temporary overlay under `~/.cache/scratch/poetry-audit-repro/`; it never copies files into `internal/`. Supplied `.go.txt` files overlay package-local `_test.go` paths, so they can exercise private helpers without adding application APIs or joining `go test ./...` as new packages. `run.py` uses Python's standard library only.

To reproduce against a separate checkout of the exact target, place only this audit asset directory under that checkout's `doc/research/poetry-audit/`. Do not run the probes against an arbitrary newer application revision and label the results as the pinned revision. Probe expected strings intentionally assert the behavior being audited, not a proposed fix.

Clean up the task-specific scratch environment afterward. Do not remove shared Go/uv caches or a reusable PostgreSQL container being used by another session. All session-owned scratch files were removed after publishing; committed results below are bounded observations rather than paths to ephemeral logs.

## Model-free probes: passed

`run.py`: five top-level probe tests, six extraction subtests; all passed. Observed values (Go `%q` escaping):

```text
TestPoetryAuditExtraction:
  br_and_empty_break:     "Alpha\n\nBeta\n\nGamma"
  paragraphs_same_output:"Alpha\n\nBeta\n\nGamma"
  pre_literal_lines:     "Alpha Beta Gamma"
  attribute_only_lines:  "Alpha Beta"
  adjacent_spans:        "AlphaBeta"
  speaker_and_direction: "Faust\n\n(tritt auf)\n\nAlpha"

TestPoetryAuditChunksAndCounts:
  hard cuts (limit=8): ["Äpfel\n\nb" "lühen oh" "ne Ende"]
  concatenation == original text; valid UTF-8; each <= 8 code points
  punctuation probe (limit=5): ["Dr." " Faus" "t geh" "t."]
  ToProto/FromProto retains multiline sentence text
  offset shift uses code points
  identical supplied token identities: count=2 with either 1 or 2 sentences

TestPoetryAuditStudyAndKWIC:
  annotated study segments reconstruct: "Äpfel\n\nblühen."
  space-normalized sentence against original unit -> whole-line fallback
  KWIC helper: "Äpfel\n\nblühen." -> "Äpfel blühen."

TestPoetryAuditCardGates:
  "Der alte Baum steht heute still im Garten.": accepted=true score=110
  one interior space replaced by LF: accepted=false score=100
  rejection: structural noise or boilerplate
  German nil parse accepted=true; supplied empty parse accepted=false
  supplied finite-root/subject skeleton accepted=true GDEXScore=0.5
  bold output: "Der alte <b>Baum</b> steht\nheute still im Garten."
  raw LF remains; no <br> added

TestPoetryAuditProviderPayload:
  allowed fields: item_id language target_language canonical_lemma upos
                  target_word example_sentence candidate_senses
  empty candidate_senses omitted (7 fields in this example)
  exact example_sentence: "Der Baum\nsteht heute still im Garten."
  SentenceHash distinguishes interior LF from ordinary space
  no HTTP request made
```

Producer probe output:

```text
producer fake-pipeline round trip: 'Äpfel\n\nblühen.'
sentence offsets: 0 14
token spans/heads: [(0, 5, 1), (7, 13, 1), (13, 14, 1)]
No model loaded; no segmentation or parse accuracy claim.
```

The producer factory supplies the sentence, tokens, morphology and dependencies. These observations measure the adapter only. The codec test also bypasses the ordinary card gate to test newline representability independently: it is **not** evidence that today's deck preparation accepts multiline examples.

## Existing regression suites: passed

Uncached targeted Go suite output, recorded before adding virtual probes:

```text
ok github.com/justin-hayes/mouseion/internal/epub         0.026s
ok github.com/justin-hayes/mouseion/internal/analyzer     0.016s
ok github.com/justin-hayes/mouseion/internal/analysis     0.006s
ok github.com/justin-hayes/mouseion/internal/selection    0.004s
ok github.com/justin-hayes/mouseion/internal/gdex         0.004s
ok github.com/justin-hayes/mouseion/internal/cardexport   0.315s
ok github.com/justin-hayes/mouseion/internal/enrichment   1.497s
ok github.com/justin-hayes/mouseion/internal/prepareddeck 0.018s
ok github.com/justin-hayes/mouseion/internal/persistence  0.009s
ok github.com/justin-hayes/mouseion/internal/webapp       9.923s
ok github.com/justin-hayes/mouseion/internal/opds         0.022s
```

Uncached integration suite output:

```text
ok github.com/justin-hayes/mouseion/internal/analysis      7.769s
ok github.com/justin-hayes/mouseion/internal/epub          0.498s
ok github.com/justin-hayes/mouseion/internal/persistence  23.560s
ok github.com/justin-hayes/mouseion/internal/prepareddeck 21.414s
ok github.com/justin-hayes/mouseion/internal/webapp       31.289s
```

Python: **91 passed, 3 skipped** (initial run 3.39s; repeat with skip reasons 2.10s). All three skips are `nlp/tests/test_real_pipelines.py:32`: opt-in `MOUSEION_TEST_STANZA_RESOURCES_DIR` and `HF_HOME` were not configured. These are real `de`/`it`/`el` pipeline probes, not failures hidden as successful model validation.

`make lint`: **0 Go issues**; ruff checks passed, 17 Python files already formatted; mypy passed for seven source files. Make reported pinned templ not locally available and used committed generated Go; no template was changed or regenerated.

The two audit Python scripts separately passed ruff check/format; Go probe text was gofmt-formatted. Local checks resolved all inventory source paths/line ranges, relative Markdown links and named regression tests. A final overlay run passed after adding the two annotated morphology words to the whitespace probe.

Full-project coverage/new-code gate and browser smoke were not run: no application code changes or implementation PR are proposed. No real-model verse analysis, real Anki-client import, visual layout test, paid translation, production data operation, or acceptance-edition acquisition was performed. Named regression tests and affected contracts are indexed by finding in [the inventory](../poetry-pipeline-audit.md).
