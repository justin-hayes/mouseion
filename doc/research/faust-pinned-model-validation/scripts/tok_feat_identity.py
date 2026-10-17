"""Feature-level identity of the pinned tokenizer input across Werther variants (scratch only).

Uses stanza's DataLoader.sentences (the processed per-sentence unit ids, feature rows and
units) for variants from tok_variants.py. Reports whether the full featurised sequences are
identical, and where they first diverge. Read-only; no model or config change.
Usage: STANZA_RESOURCES_DIR=<stanza> OMP_NUM_THREADS=2 venv/bin/python -I tok_feat_identity.py <repo> <werther.txt>
"""

import os
import re
import sys

repo, text_path = sys.argv[1], sys.argv[2]
sys.path[0:0] = [os.path.join(repo, "nlp", "src"), os.path.join(repo, "gen", "python")]

import numpy as np  # noqa: E402
from stanza.models.tokenization.data import DataLoader  # noqa: E402
from stanza.models.tokenization.trainer import Trainer  # noqa: E402

MODEL = os.path.join(os.environ["STANZA_RESOURCES_DIR"], "de", "tokenize", "combined_nocharlm.pt")
trainer = Trainer(model_file=MODEL, device="cpu")
args, vocab = trainer.args, trainer.vocab

raw = open(text_path, encoding="utf-8").read().rstrip()
blank = re.compile(r"\n[ \t\r\f\v]*\n")
variants = {
    "A_as_file": raw,
    "B_lf_only": blank.sub("\n", raw),
    "C_contract_flat": re.sub(r"[ \t\r\n\f\v]+", " ", raw).strip(),
    "D_paragraphs_kept": blank.sub("\x00", raw).replace("\n", " ").replace("\x00", "\n\n"),
}


def featurised(text):
    ds = DataLoader(args, input_text=text, vocab=vocab, evaluation=True, dictionary=None)
    rows = []
    for para in ds.sentences:
        for ids, labels, feats, units in para:
            for i in range(len(units)):
                rows.append((int(ids[i]), tuple(np.asarray(feats[i]).tolist()), units[i]))
    return rows


seqs = {k: featurised(v) for k, v in variants.items()}
names = list(seqs)
print("feat columns:", args["feat_funcs"])
for i, a in enumerate(names):
    for b in names[i + 1:]:
        sa, sb = seqs[a], seqs[b]
        if sa == sb:
            print(f"{a} vs {b}: IDENTICAL featurised sequences ({len(sa)} units)")
        else:
            first = next((k for k in range(min(len(sa), len(sb))) if sa[k] != sb[k]), min(len(sa), len(sb)))
            print(f"{a} vs {b}: differ; lengths {len(sa)}/{len(sb)}; first divergence at unit {first}: "
                  f"{sa[first:first+1]} vs {sb[first:first+1]} (unit text redacted)")
            print("   differing units (feature rows only):", sum(1 for x, y in zip(sa, sb) if x != y))
# Counts of LF-containing inputs reaching the model (should be zero: DataLoader maps LF to ' ')
print("LF units in any variant:", {k: sum(1 for _, _, u in v if u == "\n") for k, v in seqs.items()})
