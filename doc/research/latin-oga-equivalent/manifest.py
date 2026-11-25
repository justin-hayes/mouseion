"""Per-Work manifest of the OLA v0.2.0 CoNLL-U files: IDs, counts and digests only.

usage: python3 -I manifest.py <ola_inventory.json> <cts.json> <ola_source_repo.json> <conllu_dir> <out.tsv> <summary.json>
Columns: urn, class, text_source_repo, text_licence_instrument, conllu_sha256,
tokens, words, sentences, modal_citation_depth.
"""
import collections
import hashlib
import glob
import json
import os
import sys

LICENCE = {
    "canonical-latinLit": "CC-BY-SA-4.0 (license.md)",
    "Latin": "CC-BY-SA-4.0 (license.md)",
    "priapeia": "CC-BY-SA-4.0 (LICENSE.MD)",
    "additional-texts": "CC-BY-NC-SA-4.0 (LICENSE.md)",
    "digiliblt": "CC-BY-NC-SA-4.0 (LICENSE.md)",
    "csel-dev": "none in repository",
}


def repo_key(s):
    return s.rsplit("-", 1)[0] if s and s[-1].isdigit() else s


def main(inv, cts, src, cdir, out, summ):
    inv = json.load(open(inv))
    cts = json.load(open(cts))
    src = json.load(open(src))
    files = {}
    for f in glob.glob(os.path.join(cdir, "*.conllu")):
        urn = os.path.basename(f).split(".ola.", 1)[1].split(".tok01_")[0]
        files[urn] = f
    rows = []
    agg = collections.defaultdict(lambda: [0, 0])
    for urn in sorted(inv):
        x = inv[urn]
        h = hashlib.sha256(open(files[urn], "rb").read()).hexdigest()
        rk = repo_key(src[urn])
        lic = LICENCE.get(rk, "?")
        rows.append((urn, x["class_"], rk, lic, h, x["tokens"], x["words"], x["sentences"], cts[urn]["modal_depth"]))
        k = (x["class_"], "BY-SA" if "CC-BY-SA" in lic else "NC-SA" if "NC-SA" in lic else "none")
        agg[k][0] += 1
        agg[k][1] += x["words"]
    with open(out, "w") as fh:
        fh.write("\t".join(["urn", "class", "text_source_repo", "text_licence_instrument", "conllu_sha256",
                            "tokens", "words", "sentences", "modal_citation_depth"]) + "\n")
        for r in rows:
            fh.write("\t".join(str(c) for c in r) + "\n")
    s = {f"{k[0]}|{k[1]}": dict(files=v[0], words=v[1]) for k, v in sorted(agg.items())}
    json.dump(s, open(summ, "w"), indent=1)
    print(json.dumps(s, indent=1))
    print("manifest digest (sha256 of the TSV):", hashlib.sha256(open(out, "rb").read()).hexdigest())


if __name__ == "__main__":
    main(*sys.argv[1:7])
