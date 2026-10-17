"""Apply three candidate French pronominal identity policies to the observed Stanza output.

Scratch-only illustration: the rules are deliberately simple and are NOT proposals for
shared code. Usage: python3 -I identity_policies.py <stanza_results.json> <dict_report.json>
"""

import json
import sys

REFLEXIVE_LEMMAS = {"soi", "moi", "toi", "nous", "vous"}
results = json.load(open(sys.argv[1], encoding="utf-8"))
dictionary = json.load(open(sys.argv[2], encoding="utf-8"))
headwords = {key.split("|")[1] for key in dictionary["derived_rows"]}


def pronominal_headword(lemma: str, with_en: bool) -> str:
    clitic = "s'en" if with_en else ("s'" if lemma[0] in "aeéèêiîoôuh" else "se")
    return f"{clitic} {lemma}" if clitic != "s'" else f"s'{lemma}"


rows = []
for result in results:
    if result["id"].endswith("c"):
        continue  # curly copies give identical analyses; count each sentence once
    for sentence in result["producer_sentences"]:
        tokens = sentence["tokens"]
        for index, token in enumerate(tokens):
            if token["pos"] != "VERB":
                continue
            clitics = [t for t in tokens if t["head"] == index and t["pos"] == "PRON"]
            reflexive = [t for t in clitics if t["canonical_lemma"] in REFLEXIVE_LEMMAS and t["dependency"] in {"obj", "iobj", "expl:pv"}]
            inherent = [t for t in reflexive if t["dependency"] == "expl:pv"]
            with_en = any(t["canonical_lemma"] == "en" and t["dependency"].startswith("expl") for t in clitics)
            lemma = token["canonical_lemma"]
            headword = pronominal_headword(lemma, with_en)
            blanket = headword if reflexive else lemma
            selective = headword if inherent else lemma

            def lookup(identity: str) -> str:
                return "own entry" if identity in headwords else ("base entry" if identity == lemma else "no entry -> needs base-entry fallback")

            rows.append(
                {
                    "sentence": result["id"],
                    "surface": token["surface"],
                    "clitics": [f"{t['surface']}/{t['canonical_lemma']}/{t['dependency']}" for t in clitics],
                    "A_base": lemma,
                    "B_blanket": blanket,
                    "B_lookup": lookup(blanket),
                    "C_selective": selective,
                    "C_lookup": lookup(selective),
                }
            )

for row in rows:
    print(json.dumps(row, ensure_ascii=False))
for policy in ("A_base", "B_blanket", "C_selective"):
    print(policy, "distinct verb identities:", len({row[policy] for row in rows}), sorted({row[policy] for row in rows}))
