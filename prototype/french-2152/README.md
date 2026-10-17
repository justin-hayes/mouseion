# Throwaway probe for issue #2152

Never merged. [FINDINGS.md](FINDINGS.md) has the results; `out/` holds the raw observations.

Reproduce from a checkout of `6e02d180`. Use a scratch directory outside the repository; `$S` below.

```sh
UV_PROJECT_ENVIRONMENT=$S/venv uv sync --project nlp --locked --no-build      # uv 0.13.0
# Manifest (stanza-resources@569db9d…) to $S/models/resources_1.15.0.json and
# $S/models/stanza_resources/resources.json; the eight files from
# https://huggingface.co/stanfordnlp/stanza-fr/resolve/caeb7742148cb0368e4a0f2c68a0f7b09bc22880/models/<proc>/<pkg>.pt
# into $S/models/stanza_resources/fr/<proc>/<pkg>.pt; verify against out/artifacts.json.
HF_HUB_OFFLINE=1 $S/venv/bin/python -I scripts/run_stanza.py <repo> $S/models/stanza_resources $S/out
HF_HUB_OFFLINE=1 $S/venv/bin/python -I scripts/quote_probe.py $S/models/stanza_resources
HF_HUB_OFFLINE=1 $S/venv/bin/python -I scripts/quote_merge_demo.py $S/models/stanza_resources
python3 -I scripts/fetch_kaikki.py $S/dict
python3 -I scripts/inspect_dict.py <repo> $S/dict $S/out/dict_report.json
python3 -I scripts/identity_policies.py $S/out/stanza_results.json $S/out/dict_report.json
# Raw comparison: curl -r 0-2999999 the raw .gz (see FINDINGS §5), extract lang_code=fr records to JSON, then
python3 -I scripts/compare_raw_post.py $S/raw/fr_records.json $S/out/raw_vs_post.json livre chien minute abdomen gratuit
# Matcher: a scratch Go module containing a verbatim copy of internal/textmatch/match.go plus
# scripts/gomatch_main.go, run over the eligible tokens extracted from stanza_results.json.
```

Kaikki page bodies and the raw prefix are not committed; their URLs and sha256 are in `out/`.
