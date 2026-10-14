#!/usr/bin/env python3
"""Minimal stdlib text extraction from FlateDecode PDF content streams.

Only for locating quoted guideline passages; Greek text in these PDFs uses
font-specific encodings, so Greek is not recoverable here. ASCII/Latin
English text is.  Run with `python3 -I`.

  python3 -I extract_pdf_text.py guidelines.pdf > guidelines.txt
"""
import re
import sys
import zlib

STREAM = re.compile(rb"stream\r?\n(.*?)\r?\nendstream", re.S)
TOKEN = re.compile(
    rb"\((?:\\.|[^\\)])*\)|\[|\]|[-+]?\d*\.?\d+|/[^\s/\[\]()<>]+|[A-Za-z'\"*]+",
    re.S,
)
ESC = {ord("n"): b"\n", ord("r"): b"\r", ord("t"): b"\t", ord("b"): b"\b",
       ord("f"): b"\f", ord("("): b"(", ord(")"): b")", ord("\\"): b"\\"}


def unescape(raw):
    out = bytearray()
    i = 0
    while i < len(raw):
        c = raw[i]
        if c != 0x5C:
            out.append(c)
            i += 1
            continue
        i += 1
        if i >= len(raw):
            break
        c = raw[i]
        if c in ESC:
            out += ESC[c]
            i += 1
        elif 0x30 <= c <= 0x37:
            m = re.match(rb"[0-7]{1,3}", raw[i:])
            out.append(int(m.group(), 8) & 0xFF)
            i += len(m.group())
        else:
            i += 1
    return bytes(out)


def content_text(data):
    parts, ops = [], []
    for m in TOKEN.finditer(data):
        t = m.group()
        if t.startswith(b"("):
            ops.append(unescape(t[1:-1]))
        elif t in (b"TJ", b"Tj", b"'", b'"'):
            parts.append(b"".join(ops))
            ops = []
        elif t in (b"T*", b"Td", b"TD", b"Tm", b"ET"):
            parts.append(b"\n")
    return b"".join(parts).decode("latin-1")


def main(path):
    with open(path, "rb") as handle:
        raw = handle.read()
    pages = []
    for m in STREAM.finditer(raw):
        body = m.group(1)
        try:
            body = zlib.decompress(body)
        except zlib.error:
            continue
        text = content_text(body)
        if re.search(r"[A-Za-z]{3}", text):
            pages.append(text)
    sys.stdout.write("\n".join(pages))


if __name__ == "__main__":
    main(sys.argv[1])
