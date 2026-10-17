"""Scratch-only demo: re-anchor closing guillemets to the sentence they close.

Rule (illustrative, not a proposal for shared code): leading closing-quote tokens of a
sentence move to the previous sentence; if the remainder of that sentence starts with a
lowercase word (an incise such as "demanda-t-il"), the whole sentence joins the previous.
Sentence text is always sliced from the unchanged source by offsets.
Usage: python -I quote_merge_demo.py <stanza_resources_dir>
"""
import sys
import stanza
from stanza.pipeline.core import DownloadMethod

CLOSERS = {"»", "”", "›"}
nlp = stanza.Pipeline(lang="fr", model_dir=sys.argv[1], processors="tokenize,mwt",
                      package={"tokenize": "combined_nocharlm", "mwt": "combined"},
                      download_method=DownloadMethod.NONE, verbose=False)
texts = [
    "« Je suis sûr qu’il viendra ce soir. »\n\n« Il est là », dit-elle, « mais il repart demain. »\n\nIl faut partir.",
    "« Je suis sûr qu’il viendra ce soir. »\n\n« Il est là », dit-elle.",
    "« Tu viens ? » demanda-t-il. « Oui ! » Elle partit.",
    "Je vois l'homme près du marché. Il faut partir.",
]
for text in texts:
    spans = []
    for sentence in nlp(text).sentences:
        tokens = sentence.tokens
        start, end = tokens[0].start_char, tokens[-1].end_char
        lead = 0
        while lead < len(tokens) and tokens[lead].text in CLOSERS:
            lead += 1
        if spans and lead:
            prev_start, _ = spans[-1]
            if lead == len(tokens) or tokens[lead].text[:1].islower():
                spans[-1] = (prev_start, end)
                continue
            spans[-1] = (prev_start, tokens[lead - 1].end_char)
            start = tokens[lead].start_char
        spans.append((start, end))
    print([text[s:e] for s, e in spans])
