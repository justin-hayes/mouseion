"""Count AGDT sentences with/without artificial nodes, and how many of each are in UD Perseus.

Usage: python -I ellipsis_selection.py AGDT_DIR UD_CONLLU [UD_CONLLU ...]
"""
import collections
import os
import sys
import xml.etree.ElementTree as ET


def main():
    agdt_dir, ud_paths = sys.argv[1], sys.argv[2:]
    ud_ids = set()
    for p in ud_paths:
        with open(p, encoding="utf-8") as fh:
            for line in fh:
                if line.startswith("# sent_id = "):
                    ud_ids.add(line.strip()[len("# sent_id = "):])
    c = collections.Counter()
    for fname in sorted(os.listdir(agdt_dir)):
        root = ET.parse(os.path.join(agdt_dir, fname)).getroot()
        for s in root.iter("sentence"):
            ell = any(w.get("artificial") is not None for w in s.findall("word"))
            inud = f"{fname}@{s.get('id')}" in ud_ids
            c[("ellipsis" if ell else "no_ellipsis", "in_ud" if inud else "not_in_ud")] += 1
    for k in sorted(c):
        print(k, c[k])


if __name__ == "__main__":
    main()
