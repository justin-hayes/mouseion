"""Minimal, best-effort PDF text dump: inflate FlateDecode streams and collect literal strings
from text-showing operators. Only for locating words such as 'homograph' in upstream guidelines;
not a faithful text extraction (font encodings may hide text)."""
import re
import sys
import zlib

data = open(sys.argv[1], "rb").read()
out = []
for m in re.finditer(rb"stream\r?\n", data):
    start = m.end()
    end = data.find(b"endstream", start)
    if end < 0:
        continue
    chunk = data[start:end]
    try:
        raw = zlib.decompress(chunk)
    except Exception:
        continue
    for t in re.findall(rb"\((.*?)(?<!\\)\)\s*Tj|\[(.*?)\]\s*TJ", raw, re.S):
        s = t[0] or t[1]
        parts = re.findall(rb"\((.*?)(?<!\\)\)", s, re.S) if t[1] else [s]
        for p in parts:
            out.append(p.decode("latin-1", "replace"))
        out.append("\n")
sys.stdout.write("".join(out))
