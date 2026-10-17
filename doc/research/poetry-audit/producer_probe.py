"""Exercises the real producer adapter with supplied syntax, NOT a Stanza model."""

from types import SimpleNamespace as NS

from mouseion_nlp.producer import Producer, SourceDocument

text = "Äpfel\n\nblühen."
words = [
    NS(
        id=1,
        text="Äpfel",
        lemma="Apfel",
        upos="NOUN",
        feats="Number=Plur",
        head=2,
        deprel="nsubj",
        start_char=0,
        end_char=5,
    ),
    NS(
        id=2,
        text="blühen",
        lemma="blühen",
        upos="VERB",
        feats="VerbForm=Fin",
        head=0,
        deprel="root",
        start_char=7,
        end_char=13,
    ),
    NS(
        id=3,
        text=".",
        lemma=".",
        upos="PUNCT",
        feats=None,
        head=2,
        deprel="punct",
        start_char=13,
        end_char=14,
    ),
]
seen = []


def pipeline(value):
    seen.append(value)
    return NS(sentences=[NS(text=value, tokens=[NS(words=[w]) for w in words])])


result = Producer(pipeline_factory=lambda language: pipeline).analyze(
    text, "de", SourceDocument(id="unit"), run_id="audit"
)
sentence = result.sentences[0]
assert seen == [text]
assert sentence.text == text
assert text[sentence.location.start_offset : sentence.location.end_offset] == text
assert [t.head for t in sentence.tokens] == [1, 1, 1]
assert [
    text[t.location.start_offset : t.location.end_offset] for t in sentence.tokens
] == ["Äpfel", "blühen", "."]
print("producer fake-pipeline round trip:", repr(sentence.text))
print("sentence offsets:", sentence.location.start_offset, sentence.location.end_offset)
print(
    "token spans/heads:",
    [(t.location.start_offset, t.location.end_offset, t.head) for t in sentence.tokens],
)
print("No model loaded; no segmentation or parse accuracy claim.")
