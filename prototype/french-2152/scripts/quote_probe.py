"""Sentence segmentation of guillemet dialogue under realistic paragraph breaks.

Usage: python -I quote_probe.py <stanza_resources_dir>
"""
import sys
import stanza
from stanza.pipeline.core import DownloadMethod

nlp = stanza.Pipeline(lang="fr", model_dir=sys.argv[1], processors="tokenize,mwt",
                      package={"tokenize": "combined_nocharlm", "mwt": "combined"},
                      download_method=DownloadMethod.NONE, verbose=False)
variants = {
    "paragraphs": "« Je suis sûr qu’il viendra ce soir. »\n\n« Il est là », dit-elle, « mais il repart demain. »\n\nIl faut partir.",
    "narbsp": "« Je suis sûr qu’il viendra ce soir. »\n\n« Il est là », dit-elle.",
    "question_exclaim": "« Tu viens ? » demanda-t-il. « Oui ! » Elle partit.",
    "english_quotes": "\"I am sure he will come tonight.\" She left.",
}
for name, text in variants.items():
    doc = nlp(text)
    print(name, [s.text for s in doc.sentences])
