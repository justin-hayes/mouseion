"""Keep only top-level Ancient Greek entries from a raw Wiktextract JSONL.

A plain grep for "lang_code": "grc" also matches translation rows that point
at Ancient Greek from other language editions, so this filter parses each
line and checks the top-level lang_code field.

Usage (any working directory; use a scratch path for the output):
  zcat raw-wiktextract-data.jsonl.gz | python3 -I /abs/path/filter_kaikki_grc.py > grc-top.jsonl
"""
import json
import sys

kept = bad = 0
for line in sys.stdin:
    try:
        d = json.loads(line)
    except ValueError:
        bad += 1
        continue
    if d.get('lang_code') == 'grc':
        kept += 1
        sys.stdout.write(line)
sys.stderr.write(f'kept={kept} unparseable={bad}\n')
