"""Run the approved French sample through pinned Stanza 1.15.0 and Mouseion's producer.

Usage: python -I run_stanza.py <repo_root> <stanza_resources_dir> <out_dir> [sentences.json prefix]
With a sentences file (a JSON list of strings), runs those instead and prefixes output names.
"""

import json
import resource
import sys
import time
from pathlib import Path

repo, model_dir, out_dir = (Path(a) for a in sys.argv[1:4])
extra = json.loads(Path(sys.argv[4]).read_text(encoding="utf-8")) if len(sys.argv) > 4 else None
prefix = sys.argv[5] if len(sys.argv) > 5 else ""
sys.path[:0] = [str(repo / "nlp" / "src"), str(repo / "gen" / "python")]

import stanza  # noqa: E402
import torch  # noqa: E402
from mouseion_nlp.producer import Producer  # noqa: E402
from stanza.pipeline.core import DownloadMethod  # noqa: E402

torch.set_num_threads(4)

BASE = [
    "Je vois l'homme près du marché.",
    "Nous sommes d'accord sur ce point.",
    "Elle s'habille avant de partir.",
    "Il s'en va sans fermer la porte.",
    "Je le lui donne, mais elle ne m'en parle pas.",
    "Je lave la chemise après le repas.",
    "Je me lave avant le petit déjeuner.",
    "Elle se lave les mains avant de manger.",
    "Il va au marché avec sa sœur.",
    "Elle revient aux mêmes questions chaque soir.",
    "Il revient du marché avec des pommes.",
    "Le prix des pommes a augmenté.",
    "Elle boit du café et mange des fruits.",
    "Où va-t-il quand le magasin est fermé ?",
    "« Je suis sûr qu’il viendra ce soir. »",
    "« Il est là », dit-elle, « mais il repart demain. »",
    "Il faut partir.",
]
SAMPLE = [(f"S{i + 1:02d}", s) for i, s in enumerate(BASE)]
SAMPLE += [(f"S{i + 1:02d}c", s.replace("'", "’")) for i, s in enumerate(BASE[:5])]

if extra is not None:
    SAMPLE = [(f"X{i + 1:02d}", s) for i, s in enumerate(extra)]

PACKAGE = {
    "tokenize": "combined_nocharlm",
    "mwt": "combined",
    "pos": "combined_charlm",
    "lemma": "combined_nocharlm",
    "depparse": "combined_charlm",
}


def rss_mb() -> float:
    return resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024


rss_before = rss_mb()
t0 = time.perf_counter()
pipeline = stanza.Pipeline(
    lang="fr",
    model_dir=str(model_dir),
    processors="tokenize,mwt,pos,lemma,depparse",
    package=PACKAGE,
    download_method=DownloadMethod.NONE,
    verbose=False,
)
load_seconds = time.perf_counter() - t0
rss_loaded = rss_mb()
producer = Producer(pipeline_factory=lambda _language: pipeline)

results = []
latencies = []
for sid, text in SAMPLE:
    t = time.perf_counter()
    doc = pipeline(text)
    latencies.append(time.perf_counter() - t)
    raw = []
    for sentence in doc.sentences:
        for token in sentence.tokens:
            raw.append(
                {
                    "token": token.text,
                    "token_span": [token.start_char, token.end_char],
                    "source_slice": text[token.start_char : token.end_char],
                    "words": [
                        {
                            "id": w.id,
                            "text": w.text,
                            "lemma": w.lemma,
                            "upos": w.upos,
                            "feats": w.feats,
                            "head": w.head,
                            "deprel": w.deprel,
                            "word_span": [w.start_char, w.end_char],
                        }
                        for w in token.words
                    ],
                }
            )
    corpus = producer.analyze(text, "fr")
    persisted = [
        {
            "text": s.text,
            "tokens": [
                {
                    "surface": tok.surface,
                    "canonical_lemma": tok.canonical_lemma,
                    "pos": tok.pos,
                    "span": [tok.location.start_offset, tok.location.end_offset],
                    "source_slice": text[tok.location.start_offset : tok.location.end_offset],
                    "dependency": tok.dependency,
                    "head": tok.head,
                }
                for tok in s.tokens
            ],
        }
        for s in corpus.sentences
    ]
    results.append(
        {
            "id": sid,
            "text": text,
            "sentence_count": len(doc.sentences),
            "sentences": [s.text for s in doc.sentences],
            "stanza_tokens": raw,
            "producer_sentences": persisted,
        }
    )

paragraph = " ".join(text for _sid, text in SAMPLE)
t = time.perf_counter()
paragraph_doc = pipeline(paragraph)
paragraph_seconds = time.perf_counter() - t

metrics = {
    "stanza_version": stanza.__version__,
    "torch_version": torch.__version__,
    "torch_threads": torch.get_num_threads(),
    "package": PACKAGE,
    "pipeline_load_seconds": round(load_seconds, 2),
    "max_rss_mb_before_load": round(rss_before, 1),
    "max_rss_mb_after_load": round(rss_loaded, 1),
    "max_rss_mb_after_runs": round(rss_mb(), 1),
    "per_sentence_seconds": {
        "min": round(min(latencies), 3),
        "median": round(sorted(latencies)[len(latencies) // 2], 3),
        "max": round(max(latencies), 3),
        "first_call": round(latencies[0], 3),
    },
    "paragraph_chars": len(paragraph),
    "paragraph_seconds": round(paragraph_seconds, 3),
    "paragraph_sentences": [s.text for s in paragraph_doc.sentences],
}
out_dir.mkdir(parents=True, exist_ok=True)
(out_dir / f"{prefix}stanza_results.json").write_text(json.dumps(results, ensure_ascii=False, indent=1))
(out_dir / f"{prefix}stanza_metrics.json").write_text(json.dumps(metrics, ensure_ascii=False, indent=1))
print(json.dumps(metrics, ensure_ascii=False, indent=1))
