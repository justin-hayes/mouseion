"""THROWAWAY key conversion, adapted from the lexicon research at c3fdfbd7.

Unknown notation excludes a key, never silently strips it. Canonical matching
is performed separately by the real Go ancient-greek/1 implementation.
"""

import re
import unicodedata

LETTERS = dict(zip("abgdezhqiklmncoprstufxywv", "αβγδεζηθικλμνξοπρστυφχψωϝ"))
MARKS = {
    ")": "\u0313",
    "(": "\u0314",
    "/": "\u0301",
    "\\": "\u0300",
    "=": "\u0342",
    "+": "\u0308",
    "|": "\u0345",
}


def beta_to_greek(value):
    """Return headword, dictionary-native homograph suffix, unsupported chars."""
    match = re.search(r"\d+$", value)
    number = match.group() if match else ""
    value = value[: match.start()] if match else value
    out, unknown, capital, pending = [], [], False, ""
    for char in value:
        if char == "*":
            capital = True
        elif char in MARKS:
            if out and not capital:
                out[-1] += MARKS[char]
            else:
                pending += MARKS[char]
        elif char.lower() in LETTERS:
            base = LETTERS[char.lower()]
            out.append((base.upper() if capital or char.isupper() else base) + pending)
            capital, pending = False, ""
        else:
            unknown.append(char)
    return unicodedata.normalize("NFC", "".join(out)), number, unknown
