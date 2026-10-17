"""Separate the Werther 99-vs-94 difference into its causes (scratch only, read-only).

Variants of the repo Werther excerpt:
  A  as_file        : raw file (hard-wrapped lines, blank-line paragraph breaks)
  B  lf_only        : blank-line breaks removed, wrapped LFs kept (no paragraph breaks)
  C  contract_flat  : the probe's collapse (every ASCII whitespace run, incl. blank lines, -> one space)
  D  paragraphs_kept: wrapped LFs -> space, blank-line paragraph breaks -> "\\n\\n" (contract-shaped prose)

For each variant: (1) the tokenizer's own DataLoader view (paragraph count, unit count,
space/LF units after stanza's WHITESPACE_RE + consecutive-space filter), and
(2) the production sentence count from mouseion_nlp.producer._stanza_pipeline("de").
Usage: STANZA_RESOURCES_DIR=<stanza> OMP_NUM_THREADS=2 venv/bin/python -I tok_variants.py <repo> <werther.txt>
"""

import os
import re
import sys
from collections import Counter

repo, text_path = sys.argv[1], sys.argv[2]
sys.path[0:0] = [os.path.join(repo, "nlp", "src"), os.path.join(repo, "gen", "python")]

import torch  # noqa: E402

torch.set_num_threads(2)

from stanza.models.tokenization.data import DataLoader  # noqa: E402
from stanza.models.tokenization.trainer import Trainer  # noqa: E402
from mouseion_nlp.producer import _stanza_pipeline  # noqa: E402

MODEL = os.path.join(os.environ["STANZA_RESOURCES_DIR"], "de", "tokenize", "combined_nocharlm.pt")
trainer = Trainer(model_file=MODEL, device="cpu")
args, vocab = trainer.args, trainer.vocab

raw = open(text_path, encoding="utf-8").read()
blank = re.compile(r"\n[ \t\r\f\v]*\n")
variants = {
    "A_as_file": raw.rstrip(),
    "B_lf_only": blank.sub("\n", raw.rstrip()),
    "C_contract_flat": re.sub(r"[ \t\r\n\f\v]+", " ", raw).strip(),
    "D_paragraphs_kept": blank.sub("\x00", raw.rstrip()).replace("\n", " ").replace("\x00", "\n\n"),
}

nlp = _stanza_pipeline("de")
print("production pipeline processors:", [p for p in nlp.processors] if hasattr(nlp, "processors") else "n/a")
results = {}
for name, text in variants.items():
    ds = DataLoader(args, input_text=text, vocab=vocab, evaluation=True, dictionary=None)
    units = [u for para in ds.data for (c, _) in para for u in [c]]
    unit_counts = Counter("LF" if c == "\n" else ("SP" if c == " " else "other") for c in units)
    doc = nlp(text)
    sentences = [s.text for s in doc.sentences]
    results[name] = {"units": len(units), "paragraphs": len(ds.data), "sentences": len(sentences),
                     "lf_units": unit_counts["LF"], "space_units": unit_counts["SP"], "chars": len(text),
                     "text_sha256": __import__("hashlib").sha256(text.encode()).hexdigest()}
    print(name, results[name])

# Sentence-level alignment of A vs C/D by text: which A sentences are not reproduced exactly in C or D.
def norm(s):
    return re.sub(r"\s+", " ", s).strip()

for other in ("C_contract_flat", "D_paragraphs_kept", "B_lf_only"):
    a = [norm(s.text) for s in nlp(variants["A_as_file"]).sentences]
    b = [norm(s.text) for s in nlp(variants[other]).sentences]
    sa, sb = Counter(a), Counter(b)
    print(f"A vs {other}: A-only sentences {sum((sa-sb).values())}, {other}-only {sum((sb-sa).values())}")
