#!/usr/bin/env python3
"""Derive Mouseion's compact lexical index from a Kaikki JSONL dump."""

from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import os
import re
import shutil
import sqlite3
import tempfile
import unicodedata
from collections import OrderedDict
from dataclasses import dataclass, replace
from datetime import date
from pathlib import Path
from urllib.request import urlopen
from xml.etree import ElementTree

LANGUAGES = {"de", "it", "el", "fr"}
# Ancient Greek entries come from English Wiktionary too, but they are stored
# differently: one row per source entry, with the NFC source headword and no
# normalization key. Go derives the ancient-greek/1 key when it opens the
# index. The index format version tells an older index apart so it can report
# grc as unavailable.
ANCIENT_GREEK_LANGUAGE = "grc"
FRENCH_LANGUAGE = "fr"
# The LSJ (Perseus) derivation is optional: it is present only when an operator
# points --lsj-dir at the pinned PerseusDL/lexica tree. Its table and the
# metadata pin are what distinguish index format 4 from format 3.
LSJ_PIN = "56061ca127f4a2844980baffc5f2b6d1332897b3"
# tlg0032 is Xenophon in the Perseus/Cts URN space; only Xenophon citations are
# kept on an LSJ sense.
XENOPHON_URN_PREFIX = "urn:cts:greekLit:tlg0032"
# index format 3 introduced the French h_status column (ticket #2163); format 4
# adds the LSJ table and the lsj_pin metadata (ticket #2169).
INDEX_FORMAT_VERSION = 3
LSJ_INDEX_FORMAT_VERSION = 4
KAIKKI_DOWNLOAD_URL = "https://kaikki.org/dictionary/raw-wiktextract-data.jsonl.gz"
POS = {
    "adj": "ADJ",
    "adjective": "ADJ",
    "adv": "ADV",
    "adverb": "ADV",
    "noun": "NOUN",
    "proper noun": "PROPN",
    "propn": "PROPN",
    "verb": "VERB",
    "pron": "PRON",
    "pronoun": "PRON",
    "prep": "ADP",
    "preposition": "ADP",
    "art": "DET",
    "article": "DET",
    "det": "DET",
    "determiner": "DET",
    "conj": "CCONJ",
    "conjunction": "CCONJ",
    "subordinating conjunction": "SCONJ",
    "int": "INTJ",
    "intj": "INTJ",
    "interj": "INTJ",
    "interjection": "INTJ",
    "num": "NUM",
    "numeral": "NUM",
    "number": "NUM",
    "cardinal number": "NUM",
    "particle": "PART",
}
GENDERS = {
    "masculine": "Masc",
    "masc": "Masc",
    "feminine": "Fem",
    "fem": "Fem",
    "neuter": "Neut",
    "neut": "Neut",
}
IPA_ACCEPTED_TAGS = frozenset({"standard"})
# French pronunciation evidence is only trustworthy when it is untagged or
# explicitly France, Europe or Paris. Any other regional tag alone omits IPA
# rather than presenting one region's pronunciation as the standard.
FRENCH_IPA_ACCEPTED_TAGS = frozenset({"france", "europe", "paris"})
# Wiktionary marks an aspirated h only through this maintenance category. Its
# absence means unknown, never mute.
FRENCH_ASPIRATED_H_CATEGORY = "french terms with aspirated h"
# A French head template records an invariable noun in its expansion, for
# example "prix m (invariable)" or "héros m (invariable, feminine héroïne)".
FRENCH_INVARIABLE_PATTERN = re.compile(r"\binvariable\b")
OPTIONAL_SEGMENT_PATTERN = re.compile(r"\([^()]*\)")
PRINCIPAL_PARTS_PATTERN = re.compile(
    r"third-person singular present\s+(?P<present>[^,]+),\s*"
    r"past tense\s+(?P<preterite>[^,]+),\s*"
    r"past participle\s+(?P<participle>[^,)]+)"
)
SCHEMA = """
PRAGMA journal_mode = DELETE;
CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE entries (
    language TEXT NOT NULL,
    lemma TEXT NOT NULL,
    upos TEXT NOT NULL,
    senses_json TEXT NOT NULL,
    gender TEXT NOT NULL,
    article TEXT NOT NULL,
    plural TEXT NOT NULL,
    ipa TEXT NOT NULL,
    principal_parts TEXT NOT NULL,
    h_status TEXT NOT NULL,
    PRIMARY KEY (language, lemma, upos)
);
CREATE TABLE greek_entries (
    headword TEXT NOT NULL,
    upos TEXT NOT NULL,
    etymology_number TEXT NOT NULL,
    senses_json TEXT NOT NULL,
    forms_json TEXT NOT NULL
);
CREATE TABLE lsj_entries (
    entry_id TEXT NOT NULL,
    headword TEXT NOT NULL,
    senses_json TEXT NOT NULL
);
"""
INSERT_ENTRIES_SQL = (
    "INSERT INTO entries"
    "(language, lemma, upos, senses_json, gender, article, plural, ipa, principal_parts, h_status) "
    "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
)
INSERT_GREEK_ENTRIES_SQL = (
    "INSERT INTO greek_entries(headword, upos, etymology_number, senses_json, forms_json) "
    "VALUES (?, ?, ?, ?, ?)"
)
INSERT_LSJ_ENTRIES_SQL = "INSERT INTO lsj_entries(entry_id, headword, senses_json) VALUES (?, ?, ?)"


def load_german_normalization_policy() -> dict:
    path = (
        Path(__file__).resolve().parents[2]
        / "internal"
        / "canonicalization"
        / "german_post1996.json"
    )
    with path.open(encoding="utf-8") as source:
        return json.load(source)


GERMAN_NORMALIZATION_POLICY = load_german_normalization_policy()
GERMAN_V6_EQUIVALENCES = GERMAN_NORMALIZATION_POLICY.get("v6_equivalences", {})


def primary_lemma(value: str) -> str:
    for alternative in value.split("|"):
        alternative = alternative.strip()
        if alternative:
            return alternative
    return ""


def clean_lemma_edges(value: str) -> str:
    edge_cleanup = GERMAN_NORMALIZATION_POLICY["edge_cleanup"]
    preserved = set(edge_cleanup["preserve_characters"])
    categories = set(edge_cleanup["strip_unicode_categories"])

    def is_edge_decoration(character: str) -> bool:
        return character not in preserved and unicodedata.category(character)[0] in categories

    start, end = 0, len(value)
    while start < end and is_edge_decoration(value[start]):
        start += 1
    while end > start and is_edge_decoration(value[end - 1]):
        end -= 1
    return value[start:end] or value


def normalize(language: str, value: str) -> str:
    if language == "de":
        # German orthography treats ß as a distinct letter; casefold would
        # collapse it to ss and diverge from the runtime lookup key.
        value = " ".join(clean_lemma_edges(primary_lemma(value)).strip().lower().split())
        equivalences = GERMAN_NORMALIZATION_POLICY["equivalences"]
        return GERMAN_V6_EQUIVALENCES.get(value, equivalences.get(value, value))
    normalized = " ".join(unicodedata.normalize("NFC", value.strip()).casefold().split())
    if language == FRENCH_LANGUAGE:
        # The French profile keeps accents significant (où≠ou, sûr≠sur) and
        # folds the typographic apostrophe to ASCII in the canonical lemma
        # only, so aujourd'hui and aujourd’hui share one identity. This
        # mirrors Go's french profile and the producer.
        return normalized.replace("\u2019", "'")
    return normalized


def values(value: object) -> list[str]:
    if isinstance(value, str):
        return [value]
    if isinstance(value, list):
        return [item for item in value if isinstance(item, str)]
    return []


def tags_for(item: dict, sense: dict) -> list[str]:
    tags = values(item.get("tags")) + values(sense.get("tags"))
    return list(OrderedDict((tag, None) for tag in tags).keys())


def gender_for(item: dict, sense: dict) -> str:
    genders = set()
    candidates = tags_for(item, sense)
    for value in candidates:
        if value.casefold() in GENDERS:
            genders.add(GENDERS[value.casefold()])
    for category in values(item.get("categories")):
        lowered = category.casefold()
        for name, gender in GENDERS.items():
            if name in lowered:
                genders.add(gender)
    return next(iter(genders)) if len(genders) == 1 else ""


def plural_for(item: dict) -> str:
    for form in item.get("forms", []):
        if not isinstance(form, dict):
            continue
        tags = {tag.casefold() for tag in values(form.get("tags"))}
        if "plural" in tags or "pl" in tags:
            value = form.get("form")
            if isinstance(value, str) and value.strip():
                return value.strip()
    return ""


def french_invariable(item: dict) -> bool:
    """Report whether a French head template marks the noun invariable.

    Wiktionary records invariability in the head-template expansion rather
    than in an inflection form, for example "prix m (invariable)".
    """
    for template in item.get("head_templates", []):
        if not isinstance(template, dict):
            continue
        expansion = template.get("expansion")
        if isinstance(expansion, str) and FRENCH_INVARIABLE_PATTERN.search(expansion):
            return True
    return False


def plural_or_invariable(language: str, upos: str, item: dict) -> str:
    if upos != "NOUN":
        return ""
    if language == FRENCH_LANGUAGE and french_invariable(item):
        return "invariable"
    return plural_for(item)


def h_status_for(language: str, upos: str, item: dict) -> str:
    """Record French aspirated-h evidence for a noun, else nothing.

    Only Wiktionary's "French terms with aspirated h" category is evidence of
    an aspirated h. Its absence is unknown, never mute. The evidence serves the
    noun article rule, so it is recorded on NOUN rows only.
    """
    if language != FRENCH_LANGUAGE or upos != "NOUN":
        return ""
    for category in values(item.get("categories")):
        if category.casefold().strip() == FRENCH_ASPIRATED_H_CATEGORY:
            return "aspirated"
    return ""


def article_for(language: str, word: str, gender: str, forms: object) -> str:
    supplied = supplied_article_for(language, forms)
    if supplied:
        return supplied
    if language == "de":
        return {"Masc": "der", "Fem": "die", "Neut": "das"}.get(gender, "")
    if language == "el":
        return {"Masc": "ο", "Fem": "η", "Neut": "το"}.get(gender, "")
    # French articles depend on the noun's initial letter and its aspirated-h
    # evidence, so card export resolves them from the derived gender and
    # h-status rather than storing a fixed article here (ADR 0102).
    if language != "it":
        return ""
    return italian_article_for(word, gender)


def italian_article_for(word: str, gender: str) -> str:
    word = word.casefold().strip()
    if not word or not gender:
        return ""
    if word[0] in "aeiouàèéìòóù":
        return "l'"
    if gender == "Fem":
        return "la"
    if word.startswith(("z", "x", "y", "gn", "ps", "pn")) or (
        word.startswith("s") and len(word) > 1 and word[1] not in "aeiouàèéìòóù"
    ):
        return "lo"
    return "il"


def supplied_article_for(language: str, forms: object) -> str:
    article_values = {
        "de": {"der", "die", "das"},
        "it": {"il", "lo", "la", "l'"},
        "el": {"ο", "η", "το"},
    }.get(language, set())
    for form in forms if isinstance(forms, list) else []:
        if not isinstance(form, dict):
            continue
        tags = {tag.casefold() for tag in values(form.get("tags"))}
        if not tags.intersection({"article", "definite", "definite article"}):
            continue
        if "nominative" not in tags or tags.intersection({"plural", "inflected", "inflected form"}):
            continue
        value = form.get("form")
        if isinstance(value, str) and value.strip() and value.casefold().strip() in article_values:
            return value.casefold().strip()
    return ""


def ipa_tags_acceptable(language: str, tags: set[str]) -> bool:
    if language == FRENCH_LANGUAGE:
        # Untagged, or carrying at least one of France, Europe or Paris. IPA
        # that is only otherwise regional is omitted.
        return not tags or bool(tags & FRENCH_IPA_ACCEPTED_TAGS)
    return tags.issubset(IPA_ACCEPTED_TAGS)


def sound_ipa(sound: object, language: str) -> str:
    """Return one sound entry's acceptable IPA, or "" when it is not usable."""
    candidate = ""
    if isinstance(sound, dict):
        if sound.get("note"):
            return ""
        raw = sound.get("ipa")
        if isinstance(raw, str) and raw.strip():
            tags = {tag.casefold() for tag in values(sound.get("tags"))}
            if ipa_tags_acceptable(language, tags):
                candidate = raw.strip()
                if language != FRENCH_LANGUAGE:
                    candidate = OPTIONAL_SEGMENT_PATTERN.sub("", candidate).strip()
    return candidate


def ipa_for(item: dict, language: str) -> str:
    best = ""
    best_rank = None
    for sound in item.get("sounds", []):
        candidate = sound_ipa(sound, language)
        if not candidate:
            continue
        rank = 0 if candidate.startswith("/") and candidate.endswith("/") else 1
        if best_rank is None or rank < best_rank:
            best = candidate
            best_rank = rank
    return best


def principal_parts_for(language: str, upos: str, item: dict) -> str:
    # French verbs have no principal parts or conjugation summary in the
    # index: the German extractor is not reused, and nothing is invented.
    if language in {"el", FRENCH_LANGUAGE} or upos != "VERB":
        return ""
    for template in item.get("head_templates", []):
        if not isinstance(template, dict):
            continue
        expansion = template.get("expansion")
        if not isinstance(expansion, str):
            continue
        match = PRINCIPAL_PARTS_PATTERN.search(expansion)
        if not match:
            continue
        parts = [
            match.group("present").strip(),
            match.group("preterite").strip(),
            match.group("participle").strip(),
        ]
        if all(parts):
            return " · ".join(parts)
    return ""


def upos_for(item: dict, sense: dict) -> list[str]:
    raw_pos = str(item.get("pos", "")).casefold()
    if raw_pos in {"conj", "conjunction"}:
        tags = {tag.casefold() for tag in tags_for(item, sense)}
        if "subordinating" in tags:
            return ["SCONJ"]
        if "coordinating" in tags:
            return ["CCONJ"]
        # Raw conjunction entries do not always carry a qualifier. Keep both
        # analyzer identities available rather than dropping one.
        return ["CCONJ", "SCONJ"]
    upos = POS.get(raw_pos)
    # Keep otherwise eligible meanings when Wiktionary has no POS or a POS we
    # do not map to analyzer identities. The empty index key is deliberately
    # not an invented UPOS; retrieval labels it as weaker lemma-only evidence.
    return [upos] if upos else [""]


def sense_from(item: dict, raw: dict, upos: str) -> dict | None:
    glosses = [gloss.strip() for gloss in values(raw.get("glosses")) if gloss.strip()]
    if not glosses:
        return None
    examples = []
    for example in raw.get("examples", []):
        if isinstance(example, dict) and isinstance(example.get("text"), str):
            examples.append(example["text"])
        elif isinstance(example, str):
            examples.append(example)
    tags = tags_for(item, raw)
    phrase = raw.get("phrase") if isinstance(raw.get("phrase"), str) else ""
    if not phrase and " " in item.get("word", ""):
        phrase = item["word"]
    language = item["lang_code"]
    # French page categories leak gender onto non-noun parts of speech, so the
    # derivation records gender on NOUN rows only.
    gender = "" if language == FRENCH_LANGUAGE and upos != "NOUN" else gender_for(item, raw)
    return {
        "Gloss": glosses[0].strip(),
        "Examples": examples,
        "Topics": values(raw.get("topics")),
        "Tags": tags,
        "Phrase": phrase.strip(),
        "Gender": gender,
        "Article": article_for(language, item["word"], gender, item.get("forms"))
        if upos == "NOUN"
        else "",
        "Plural": plural_or_invariable(language, upos, item),
        "IPA": ipa_for(item, language),
    }


def open_input(path: Path):
    if path.suffix == ".gz":
        return gzip.open(path, mode="rt", encoding="utf-8")
    return path.open(encoding="utf-8")


def file_sha256(path: Path) -> str:
    open_binary = gzip.open if path.suffix == ".gz" else Path.open
    digest = hashlib.sha256()
    with open_binary(path, "rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def download_cache_path() -> Path:
    cache_root = os.environ.get("XDG_CACHE_HOME") or str(Path.home() / ".cache")
    return Path(cache_root) / "mouseion" / "raw-wiktextract-data.jsonl.gz"


def download_input(force: bool) -> Path:
    output = download_cache_path()
    if output.exists() and not force:
        return output
    if not KAIKKI_DOWNLOAD_URL.startswith(("http://", "https://")):
        # S310: urlopen also accepts file: and custom schemes, so reject
        # anything but the fixed http(s) dump URL before opening it.
        raise RuntimeError(f"refusing to download from a non-http(s) URL: {KAIKKI_DOWNLOAD_URL}")
    output.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(
        dir=output.parent, prefix=output.name + ".", suffix=".tmp"
    )
    temporary_path = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "wb") as destination, urlopen(KAIKKI_DOWNLOAD_URL) as source:
            shutil.copyfileobj(source, destination)
        temporary_path.replace(output)
    finally:
        temporary_path.unlink(missing_ok=True)
    return output


def new_entry() -> dict:
    return {
        "senses": [],
        "gender": "",
        "genders": set(),
        "article": "",
        "article_from_source": False,
        "fallback_article": "",
        "plural": "",
        "ipa": "",
        "principal_parts": "",
        "h_status": "",
    }


def record_sense(
    entries: dict[tuple[str, str, str], dict],
    key: tuple[str, str, str],
    sense: dict,
    item: dict,
    upos: str,
) -> None:
    language = key[0]
    entry = entries.setdefault(key, new_entry())
    identity = json.dumps(sense, ensure_ascii=False, sort_keys=True)
    if not any(
        json.dumps(existing, ensure_ascii=False, sort_keys=True) == identity
        for existing in entry["senses"]
    ):
        entry["senses"].append(sense)
    if sense["Gender"]:
        entry["genders"].add(sense["Gender"])
    entry["gender"] = next(iter(entry["genders"])) if len(entry["genders"]) == 1 else ""
    source_article = supplied_article_for(language, item.get("forms"))
    if source_article:
        entry["article"] = source_article
        entry["article_from_source"] = True
    elif not entry["fallback_article"]:
        entry["fallback_article"] = sense["Article"]
    if not entry["article_from_source"]:
        entry["article"] = entry["fallback_article"] if entry["gender"] else ""
    entry["plural"] = entry["plural"] or sense["Plural"]
    entry["ipa"] = entry["ipa"] or ipa_for(item, language)
    entry["principal_parts"] = entry["principal_parts"] or principal_parts_for(language, upos, item)
    entry["h_status"] = entry["h_status"] or h_status_for(language, upos, item)


def senses_for_item(item: dict):
    for raw_sense in item.get("senses", []):
        if not isinstance(raw_sense, dict):
            continue
        for upos in upos_for(item, raw_sense):
            sense = sense_from(item, raw_sense, upos)
            if sense is not None:
                yield upos, sense


def indexed_key(item: dict) -> tuple[str, str] | None:
    language = item.get("lang_code")
    if language not in LANGUAGES:
        return None
    word = item.get("word")
    if not isinstance(word, str) or not word.strip():
        return None
    return language, normalize(language, word)


def collect_entries(input_path: Path) -> dict[tuple[str, str, str], dict]:
    entries: dict[tuple[str, str, str], dict] = {}
    with open_input(input_path) as source:
        for line in source:
            if not line.strip():
                continue
            item = json.loads(line)
            key = indexed_key(item)
            if key is None:
                continue
            language, lemma = key
            for upos, sense in senses_for_item(item):
                record_sense(entries, (language, lemma, upos), sense, item, upos)
    return entries


def greek_etymology_number(item: dict) -> str:
    value = item.get("etymology_number")
    if isinstance(value, bool):
        return ""
    if isinstance(value, int):
        return str(value)
    if isinstance(value, str) and value.strip():
        return value.strip()
    return ""


def greek_forms(item: dict) -> list[str]:
    forms = []
    for form in item.get("forms", []):
        value = form.get("form") if isinstance(form, dict) else form
        if isinstance(value, str) and value.strip():
            forms.append(unicodedata.normalize("NFC", value.strip()))
    return forms


def greek_rows_for_item(item: dict) -> list[tuple[str, str, str, str, str]]:
    word = item.get("word")
    if not isinstance(word, str) or not word.strip():
        return []
    headword = unicodedata.normalize("NFC", word.strip())
    grouped: dict[str, list[dict]] = {}
    for upos, sense in senses_for_item(item):
        grouped.setdefault(upos, []).append(sense)
    if not grouped:
        return []
    forms_json = json.dumps(greek_forms(item), ensure_ascii=False, separators=(",", ":"))
    etymology_number = greek_etymology_number(item)
    rows = []
    for upos in sorted(grouped):
        senses_json = json.dumps(grouped[upos], ensure_ascii=False, separators=(",", ":"))
        rows.append((headword, upos, etymology_number, senses_json, forms_json))
    return rows


def collect_greek_entries(input_path: Path) -> list[tuple[str, str, str, str, str]]:
    # One row per source entry (never merged by key): two Wiktionary homographs
    # that share a headword and part of speech stay separate rows.
    rows: list[tuple[str, str, str, str, str]] = []
    with open_input(input_path) as source:
        for line in source:
            if not line.strip():
                continue
            item = json.loads(line)
            if item.get("lang_code") != ANCIENT_GREEK_LANGUAGE:
                continue
            rows.extend(greek_rows_for_item(item))
    return sorted(rows)


# ---- LSJ (Perseus TEI P4) derivation --------------------------------------
#
# The Perseus LSJ files are TEI P4: each main entry is an <entryFree type="main">
# whose key is Beta Code. Senses are nested <sense> elements carrying the source
# numbering (n) and nesting level; English glosses are <tr> elements and passage
# citations are <bibl> elements linked to a CTS URN. The native entry and sense
# ids and the hierarchy are preserved; only Xenophon citations are kept.

LSJ_LETTERS = {
    "a": "α",
    "b": "β",
    "g": "γ",
    "d": "δ",
    "e": "ε",
    "z": "ζ",
    "h": "η",
    "q": "θ",
    "i": "ι",
    "k": "κ",
    "l": "λ",
    "m": "μ",
    "n": "ν",
    "c": "ξ",
    "o": "ο",
    "p": "π",
    "r": "ρ",
    "s": "σ",
    "t": "τ",
    "u": "υ",
    "f": "φ",
    "x": "χ",
    "y": "ψ",
    "w": "ω",
    "v": "ϝ",
}
LSJ_MARKS = {
    ")": "\u0313",
    "(": "\u0314",
    "/": "\u0301",
    "\\": "\u0300",
    "=": "\u0342",
    "+": "\u0308",
    "|": "\u0345",
}


def strip_trailing_digits(value: str) -> str:
    """Drop a Beta Code homograph's trailing digits from its key."""
    end = len(value)
    while end > 0 and value[end - 1] in "0123456789":
        end -= 1
    return value[:end]


def beta_to_greek(value: str) -> tuple[str, list[str]]:
    """Return (NFC Greek, unknown characters) for a Perseus Beta Code key."""
    out: list[str] = []
    unknown: list[str] = []
    capital = False
    pending = ""
    for character in value:
        if character == "*":
            capital = True
            continue
        if character in LSJ_MARKS:
            if out and not capital:
                out[-1] = out[-1] + LSJ_MARKS[character]
            else:
                pending += LSJ_MARKS[character]
            continue
        base = LSJ_LETTERS.get(character.lower())
        if base is None:
            unknown.append(character)
            continue
        if capital or character.isupper():
            base = base.upper()
            capital = False
        out.append(base + pending)
        pending = ""
    return unicodedata.normalize("NFC", "".join(out)), unknown


def lsj_headword(key: str) -> tuple[str, bool]:
    """Return (NFC headword, admitted). The trailing homograph digit is dropped;
    the homograph is preserved by the row's native entry id, never by numbering."""
    key = strip_trailing_digits(key)
    greek, unknown = beta_to_greek(key)
    headword = greek.strip()
    return headword, bool(headword) and not unknown


def element_text(element) -> str:
    return " ".join("".join(element.itertext()).split())


def lsj_citation(bibl) -> str:
    """Return the Xenophon citation text for one <bibl>, or the empty string."""
    urn = bibl.get("n") or ""
    if not urn.startswith(XENOPHON_URN_PREFIX):
        return ""
    parts = []
    for tag in ("author", "title", "biblScope"):
        child = bibl.find(tag)
        if child is not None:
            text = element_text(child)
            if text:
                parts.append(text)
    return " ".join(parts) if parts else urn


# LSJ also marks cognates with <tr>: after a language abbreviation, usually in
# the closing "(Cf. Skt. ..., Lat. ...)" etymology. Those are not English glosses.
LSJ_COGNATE_LANGUAGE = re.compile(
    r"\b(?:Skt|Lat|Lith|Goth|Engl|OE|OHG|Slav|Arm|OIr|Umbr|Osc|Russ|Icel|Av|Hitt)\.?\s*$"
)
LSJ_COGNATE_LANGUAGE_ANYWHERE = re.compile(
    r"\b(?:Skt|Lat|Lith|Goth|Engl|OE|OHG|Slav|Arm|OIr|Umbr|Osc|Russ|Icel|Av|Hitt)\."
)
LSJ_ETYMOLOGY_NOTE = re.compile(r"\(\s*Cf\.")
LSJ_GLOSS_TRIM = " \t\n,;:"


def open_parenthesis(flow: str) -> str:
    """Return the text of the innermost still-open parenthesis, or ''."""
    starts: list[int] = []
    for index, character in enumerate(flow):
        if character == "(":
            starts.append(index)
        elif character == ")" and starts:
            starts.pop()
    return flow[starts[0] :] if starts else ""


def is_cognate(flow: str) -> bool:
    """Report whether a <tr> preceded by flow is a cognate, not an English gloss."""
    if LSJ_COGNATE_LANGUAGE.search(flow):
        return True
    parenthesis = open_parenthesis(flow)
    # A capitalised "(Cf." opens LSJ's closing etymology note.
    return bool(LSJ_ETYMOLOGY_NOTE.match(parenthesis)) or bool(
        LSJ_COGNATE_LANGUAGE_ANYWHERE.search(parenthesis)
    )


def clean_gloss(text: str) -> str:
    """Trim list punctuation; a gloss with no letters is dropped."""
    text = text.strip(LSJ_GLOSS_TRIM)
    return text if any(character.isalpha() for character in text) else ""


def collect_gloss(tr, glosses: list[str], flow: list[str]) -> None:
    """Append one <tr> as a gloss unless it is a cognate or only punctuation."""
    text = element_text(tr)
    cleaned = "" if is_cognate("".join(flow)) else clean_gloss(text)
    if cleaned:
        glosses.append(cleaned)
    flow.append(text)


def collect_sense_content(
    element, glosses: list[str], citations: list[str], flow: list[str] | None = None
) -> None:
    # flow accumulates the sense's own text in document order so each <tr> can be
    # judged by what precedes it.
    if flow is None:
        flow = []
    if element.text:
        flow.append(element.text)
    for child in element:
        # A nested sense is derived on its own; only its tail belongs here.
        if child.tag == "tr":
            collect_gloss(child, glosses, flow)
        elif child.tag == "bibl":
            citation = lsj_citation(child)
            if citation:
                citations.append(citation)
        elif child.get("lang") == "greek":
            # Beta Code writes rough breathing as "(", so Greek text stays out of
            # the flow that parenthesis tracking reads.
            collect_sense_content(child, glosses, citations, [])
        elif child.tag != "sense":
            collect_sense_content(child, glosses, citations, flow)
        if child.tail:
            flow.append(child.tail)


def dedupe(values: list[str]) -> list[str]:
    return list(OrderedDict((value, None) for value in values).keys())


def lsj_sense(element) -> dict:
    glosses: list[str] = []
    citations: list[str] = []
    collect_sense_content(element, glosses, citations)
    sense = {
        "id": (element.get("id") or "").strip(),
        "glosses": dedupe(glosses),
        "citations": dedupe(citations),
    }
    if number := (element.get("n") or "").strip():
        sense["n"] = number
    try:
        sense["level"] = int(element.get("level") or "0")
    except ValueError:
        sense["level"] = 0
    children = [lsj_sense(child) for child in element if child.tag == "sense"]
    if children:
        sense["senses"] = children
    return sense


def lsj_rows_for_file(path: Path) -> list[tuple[str, str, str]]:
    rows: list[tuple[str, str, str]] = []
    # The LSJ files are operator-supplied and digest-pinned (ADR 0101), never
    # fetched from an untrusted source, so entity-expansion attacks are out of
    # scope here.
    for _, element in ElementTree.iterparse(path, events=("end",)):  # noqa: S314
        if element.tag not in ("entryFree", "entry") or element.get("type") != "main":
            continue
        entry_id = (element.get("id") or "").strip()
        headword, admitted = lsj_headword(element.get("key") or "")
        senses = [lsj_sense(child) for child in element if child.tag == "sense"]
        element.clear()
        if not admitted or not entry_id or not senses:
            continue
        senses_json = json.dumps(senses, ensure_ascii=False, separators=(",", ":"))
        rows.append((entry_id, headword, senses_json))
    return rows


def collect_lsj_entries(lsj_dir: Path) -> list[tuple[str, str, str]]:
    rows: list[tuple[str, str, str]] = []
    for path in sorted(lsj_dir.glob("grc.lsj.perseus-eng*.xml")):
        rows.extend(lsj_rows_for_file(path))
    return sorted(rows)


@dataclass(frozen=True)
class SourceMetadata:
    """The dump provenance recorded in the index's metadata table."""

    dump_date: str = ""
    extraction_date: str = ""
    wiktextract_commit: str = ""
    # The sha256 of the raw Kaikki/Wiktextract input file. `derive` computes it
    # from the input when a caller does not supply one.
    source_sha256: str = ""


@dataclass(frozen=True)
class LsjSource:
    """The pinned PerseusDL/lexica LSJ TEI directory and its revision pin."""

    directory: Path
    pin: str = LSJ_PIN


@dataclass
class DerivedRows:
    """The three row sets a derivation collects before writing the index."""

    entries: dict[tuple[str, str, str], dict]
    greek_entries: list[tuple[str, str, str, str, str]]
    lsj_entries: list[tuple[str, str, str]]


def metadata_rows(provider_version: str, source: SourceMetadata, lsj_pin: str) -> dict[str, str]:
    rows = {
        "index_format": str(LSJ_INDEX_FORMAT_VERSION if lsj_pin else INDEX_FORMAT_VERSION),
        "provider_version": provider_version,
        "license": "Wiktionary-derived data: CC BY-SA 4.0 / GFDL",
        "generated_at": date.today().isoformat(),
        "source": "Kaikki.org Wiktextract enwiktionary JSONL",
        "attribution": "Wiktionary contributors; CC BY-SA 4.0 / GFDL",
        "dump_date": source.dump_date,
        "extraction_date": source.extraction_date,
        "wiktextract_commit": source.wiktextract_commit,
        "source_sha256": source.source_sha256,
    }
    if lsj_pin:
        rows["lsj_pin"] = lsj_pin
    return rows


def entry_rows(entries: dict[tuple[str, str, str], dict]):
    for (language, lemma, upos), entry in sorted(entries.items()):
        yield (
            language,
            lemma,
            upos,
            json.dumps(entry["senses"], ensure_ascii=False, separators=(",", ":")),
            entry["gender"],
            entry["article"],
            entry["plural"],
            entry["ipa"],
            entry["principal_parts"],
            entry["h_status"],
        )


def write_index(
    output_path: Path,
    rows: DerivedRows,
    provider_version: str,
    source: SourceMetadata,
    lsj_pin: str,
) -> None:
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(
        dir=output_path.parent, prefix=output_path.name + ".", suffix=".tmp", delete=False
    ) as temporary:
        temporary_path = Path(temporary.name)
    try:
        connection = sqlite3.connect(temporary_path)
        connection.executescript(SCHEMA)
        connection.executemany(
            "INSERT INTO metadata(key, value) VALUES (?, ?)",
            metadata_rows(provider_version, source, lsj_pin).items(),
        )
        connection.executemany(INSERT_ENTRIES_SQL, entry_rows(rows.entries))
        connection.executemany(INSERT_GREEK_ENTRIES_SQL, rows.greek_entries)
        connection.executemany(INSERT_LSJ_ENTRIES_SQL, rows.lsj_entries)
        connection.commit()
        connection.execute("PRAGMA journal_mode = OFF")
        connection.close()
        temporary_path.chmod(0o644)
        temporary_path.replace(output_path)
    finally:
        temporary_path.unlink(missing_ok=True)


def derive(
    input_path: Path,
    output_path: Path,
    provider_version: str,
    source: SourceMetadata | None = None,
    lsj: LsjSource | None = None,
) -> None:
    source = source or SourceMetadata()
    if not source.source_sha256:
        source = replace(source, source_sha256=file_sha256(input_path))
    rows = DerivedRows(
        entries=collect_entries(input_path),
        greek_entries=collect_greek_entries(input_path),
        lsj_entries=collect_lsj_entries(lsj.directory) if lsj else [],
    )
    write_index(
        output_path,
        rows,
        provider_version,
        source,
        lsj.pin if lsj else "",
    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--input", type=Path, help="raw Kaikki/Wiktextract JSONL(.gz)")
    source.add_argument(
        "--download", action="store_true", help="download the raw Kaikki dump first"
    )
    parser.add_argument("--force-download", action="store_true", help="redownload the Kaikki dump")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--provider-version")
    parser.add_argument("--dump-date", default="")
    parser.add_argument("--extraction-date", default=date.today().isoformat())
    parser.add_argument("--wiktextract-commit", default="")
    parser.add_argument("--lsj-dir", type=Path, help="PerseusDL/lexica grc/lsj TEI directory")
    parser.add_argument("--lsj-pin", default=LSJ_PIN, help="pinned PerseusDL/lexica revision")
    args = parser.parse_args(argv)
    if args.force_download and not args.download:
        parser.error("--force-download requires --download")
    if args.lsj_dir and not args.lsj_dir.is_dir():
        parser.error(f"--lsj-dir is not a directory: {args.lsj_dir}")
    input_path = args.input
    if args.download:
        try:
            input_path = download_input(args.force_download)
        except (OSError, RuntimeError) as error:
            parser.error(f"could not download Kaikki dump: {error}")
    provider_version = (
        args.provider_version
        or f"dump={args.dump_date or 'unknown'};extraction={args.extraction_date};wiktextract={args.wiktextract_commit or 'unknown'}"
    )
    derive(
        input_path,
        args.output,
        provider_version,
        SourceMetadata(
            dump_date=args.dump_date,
            extraction_date=args.extraction_date,
            wiktextract_commit=args.wiktextract_commit,
        ),
        LsjSource(directory=args.lsj_dir, pin=args.lsj_pin) if args.lsj_dir else None,
    )
    print(f"Wrote {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
