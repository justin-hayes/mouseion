"""Producer run on the contract-shaped Werther prose variant D (scratch only).

D keeps the blank-line paragraph breaks as "\\n\\n" (a block boundary) and turns wrapped LFs
into spaces, which is what the #2147 contract produces for prose paragraphs. Output uses the
run_prose.py shape so export_gate_corpus.py can consume it. Writes a new file; never overwrites.
Usage: STANZA_RESOURCES_DIR=<stanza> OMP_NUM_THREADS=2 venv/bin/python -I run_prose_d.py <repo> <out.json>
"""

import json
import os
import re
import sys

repo, out_path = sys.argv[1], sys.argv[2]
sys.path[0:0] = [os.path.join(repo, "nlp", "src"), os.path.join(repo, "gen", "python")]

import torch  # noqa: E402

torch.set_num_threads(int(os.environ.get("OMP_NUM_THREADS", "2")))

from mouseion_nlp.producer import Producer, SourceDocument  # noqa: E402

raw = open(os.path.join(repo, "nlp", "testdata", "german_prose_werther_excerpt.txt"), encoding="utf-8").read().rstrip()
blank = re.compile(r"\n[ \t\r\f\v]*\n")
text = blank.sub("\x00", raw).replace("\n", " ").replace("\x00", "\n\n")
text = re.sub(r"[ \t\r\f\v]+", " ", text)
producer = Producer()
corpus = producer.analyze(text, "de", SourceDocument(id="probe-prose-d", title="werther"), run_id="probe-prose-d")
sents = [{"text": s.text, "start": s.location.start_offset, "end": s.location.end_offset,
          "equals_slice": s.text == text[s.location.start_offset:s.location.end_offset],
          "token_detail": [{"surface": t.surface, "raw_lemma": t.raw_lemma, "canonical_lemma": t.canonical_lemma,
                            "pos": t.pos, "dep": t.dependency, "head": t.head, "morph": dict(t.morphology),
                            "span": [t.location.start_offset, t.location.end_offset]} for t in s.tokens]}
         for s in corpus.sentences]
json.dump({"werther_paragraphs_kept": {"text": text, "sentences": sents}}, open(out_path, "x", encoding="utf-8"),
          ensure_ascii=False, indent=1)
print("sentences", len(sents), "equal_to_slice", sum(s["equals_slice"] for s in sents),
      "lf_in_text", sum(s["text"].count("\n") for s in sents), "source_text_lf", text.count("\n"))
