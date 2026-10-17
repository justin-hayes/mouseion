"""Extra isolated probes: the same line analysed alone vs inside its group (scratch only).

Usage: venv python -I run_extra.py <segmentation.json> <out.json> <repo>
"""

import json
import os
import sys

seg_path, out_path, repo = sys.argv[1], sys.argv[2], sys.argv[3]
sys.path[0:0] = [os.path.join(repo, "nlp", "src"), os.path.join(repo, "gen", "python")]

from mouseion_nlp.producer import Producer, SourceDocument  # noqa: E402

units = json.load(open(seg_path, encoding="utf-8"))["units"]
pairs = [g for g in units[1]["groups"] if "Sie zu befriedigen ist schwer" in g["text"]]
assert len(pairs) == 1
group_text = pairs[0]["text"]
pair_lines = group_text.split("\n")
# Lines 194-195 of the source: the en-dash line and the "Was fällt" line.
line_194 = [l for l in pair_lines if l.startswith("Sie zu befriedigen")][0]
line_195 = "Was fällt euch an? Entzückung oder Schmerzen?"
assert line_195 in pair_lines

producer = Producer()
cases = {
    "isolated_line_195": line_195,
    "lines_194_195_only": line_194 + "\n" + line_195,
    "line_195_in_group_38": group_text,
}
out = {}
for label, text in cases.items():
    corpus = producer.analyze(text, "de", SourceDocument(id="probe-extra", title="pg21000"), run_id="probe-extra")
    out[label] = {
        "text": text,
        "sentences": [
            {"text": s.text, "start": s.location.start_offset, "end": s.location.end_offset,
             "equals_slice": s.text == text[s.location.start_offset:s.location.end_offset],
             "tokens": [t.surface for t in s.tokens],
             "token_detail": [
                 {"surface": t.surface, "raw_lemma": t.raw_lemma, "canonical_lemma": t.canonical_lemma,
                  "pos": t.pos, "dep": t.dependency, "head": t.head, "morph": dict(t.morphology),
                  "span": [t.location.start_offset, t.location.end_offset]}
                 for t in s.tokens
             ]}
            for s in corpus.sentences
        ],
    }
    print(label, len(corpus.sentences), [s.text[:60] for s in corpus.sentences][:4])
json.dump(out, open(out_path, "w", encoding="utf-8"), ensure_ascii=False, indent=1)
