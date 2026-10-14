"""Shared helpers for the Anabasis Book 1 probes (run with python3 -I).

WORK (env, default ./work) holds the pinned upstream files in raw/ and generated files in out/.
Neither is committed: see the report for the fetch commands and SHA256 values.
"""
import os
import re
import unicodedata as ud
import xml.etree.ElementTree as ET

TEI = "{http://www.tei-c.org/ns/1.0}"
WORK = os.environ.get("WORK", os.path.abspath("work"))
RAW = os.path.join(WORK, "raw")
OUT = os.path.join(WORK, "out")

APOS = {"'", "’", "ʼ", "᾽", "᾿", "̓"}


def is_word_char(ch):
    cat = ud.category(ch)
    return cat[0] in "LM" or ch in APOS


def tokenize(text):
    """Return (token, start, end) in code points; words are letter/mark runs, every other non-space char is a token."""
    out = []
    i = 0
    n = len(text)
    while i < n:
        ch = text[i]
        if ch.isspace():
            i += 1
            continue
        if is_word_char(ch):
            j = i
            while j < n and is_word_char(text[j]):
                j += 1
            out.append((text[i:j], i, j))
            i = j
        else:
            out.append((ch, i, i + 1))
            i += 1
    return out


def fold_key(s):
    """Mouseion ancient-greek v1 profile key: NFC, Unicode lowercase, final sigma -> sigma, NFC.
    (Apostrophe-like elision marks are NOT touched; reported separately.)"""
    s = ud.normalize("NFC", s).lower().replace("ς", "σ")
    return ud.normalize("NFC", s)


def loose_key(s):
    """Diagnostic only: fold_key with elision/apostrophe variants and breathing-free comparison disabled."""
    k = fold_key(s)
    k = "".join(c for c in ud.normalize("NFD", k) if c not in APOS)
    return ud.normalize("NFC", k)


def codepoints(s):
    return len(s)


def ws_units(s):
    return len(s.split())
