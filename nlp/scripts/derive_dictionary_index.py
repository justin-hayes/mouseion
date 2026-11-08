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
from collections.abc import Iterator
from dataclasses import dataclass, field, replace
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
# metadata pin are what distinguish an LSJ index from a Kaikki-only one.
LSJ_PIN = "56061ca127f4a2844980baffc5f2b6d1332897b3"
# tlg0032 is Xenophon in the Perseus/Cts URN space; only Xenophon citations are
# kept on an LSJ sense.
XENOPHON_URN_PREFIX = "urn:cts:greekLit:tlg0032"
# index format 3 introduced the French h_status column (ticket #2163); format 4
# adds the LSJ table and the lsj_pin metadata (ticket #2169); format 5 adds the
# LSJ sub-entry classification that keeps a phrase sub-entry or a redirect stub
# from counting as a homograph (ticket #2214); format 6 (and non-LSJ format 4)
# adds French form-of links and headword membership for lemma-risk review
# (ticket #2244); format 7 (and non-LSJ format 5) adds the word-to-multi-word
# idiom lookup (ticket #2361).
INDEX_FORMAT_VERSION = 5
LSJ_INDEX_FORMAT_VERSION = 7
# Idiom candidates are two to four words. Longer expressions are not idioms for
# the matcher; shorter ones are a single lemma already.
IDIOM_MIN_WORDS = 2
IDIOM_MAX_WORDS = 4
# The derived index records which evidence source supplied an idiom entry.
# Wiktionary entries keep the index's provider version; Mouseion's reviewed list
# carries its own version so a refresh can be told apart from dictionary data.
WIKTIONARY_IDIOM_SOURCE = "wiktionary"
REVIEWED_IDIOM_SOURCE = "reviewed-idioms"
DEFAULT_REVIEWED_IDIOM_VERSION = "reviewed-idioms/1"
# Wiktionary's part of speech for proper nouns. Such entries are never idioms.
WIKTIONARY_NAME_POS = "name"
# Wiktionary's gloss for a sense that only points at the literal reading of a
# multi-word entry, as in "Used other than figuratively or idiomatically: see …".
LITERAL_USE_MARKER = re.compile(r"used other than figuratively or idiomatically", re.IGNORECASE)
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
    senses_json TEXT NOT NULL,
    subordinate INTEGER NOT NULL
);
CREATE TABLE french_forms (
    form TEXT NOT NULL,
    upos TEXT NOT NULL,
    target TEXT NOT NULL,
    participle INTEGER NOT NULL,
    evidence_id TEXT NOT NULL
);
CREATE INDEX french_forms_lookup ON french_forms(form, upos);
CREATE TABLE french_headwords (
    lemma TEXT NOT NULL,
    upos TEXT NOT NULL,
    PRIMARY KEY (lemma, upos)
);
CREATE TABLE idiom_entries (
    language TEXT NOT NULL,
    headword TEXT NOT NULL,
    upos TEXT NOT NULL,
    words_json TEXT NOT NULL,
    senses_json TEXT NOT NULL,
    source TEXT NOT NULL,
    version TEXT NOT NULL,
    PRIMARY KEY (language, headword, upos, source)
);
CREATE TABLE idiom_words (
    language TEXT NOT NULL,
    word TEXT NOT NULL,
    headword TEXT NOT NULL,
    upos TEXT NOT NULL,
    source TEXT NOT NULL,
    PRIMARY KEY (language, word, headword, upos, source)
);
CREATE INDEX idiom_words_lookup ON idiom_words(language, word);
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
INSERT_LSJ_ENTRIES_SQL = (
    "INSERT INTO lsj_entries(entry_id, headword, senses_json, subordinate) VALUES (?, ?, ?, ?)"
)
INSERT_FRENCH_FORMS_SQL = (
    "INSERT INTO french_forms(form, upos, target, participle, evidence_id) VALUES (?, ?, ?, ?, ?)"
)
INSERT_FRENCH_HEADWORDS_SQL = "INSERT INTO french_headwords(lemma, upos) VALUES (?, ?)"
INSERT_IDIOM_ENTRIES_SQL = (
    "INSERT INTO idiom_entries(language, headword, upos, words_json, senses_json, source, version) "
    "VALUES (?, ?, ?, ?, ?, ?, ?)"
)
INSERT_IDIOM_WORDS_SQL = (
    "INSERT INTO idiom_words(language, word, headword, upos, source) VALUES (?, ?, ?, ?, ?)"
)


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


def parsed_items(input_path: Path) -> Iterator[dict]:
    """Yield each non-empty JSON object from a Kaikki dump."""
    with open_input(input_path) as source:
        for line in source:
            if line.strip():
                yield json.loads(line)


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
    for item in parsed_items(input_path):
        key = indexed_key(item)
        if key is None:
            continue
        language, lemma = key
        for upos, sense in senses_for_item(item):
            record_sense(entries, (language, lemma, upos), sense, item, upos)
    return entries


def idiom_headword_words(value: str) -> list[str]:
    """Return the whitespace-separated words of a multi-word headword."""
    return unicodedata.normalize("NFC", value).split()


def sense_is_form_or_alt_of(item: dict, raw_sense: dict) -> bool:
    """Report whether a sense only points at an inflected or alternative form."""
    tags = {tag.casefold() for tag in tags_for(item, raw_sense)}
    return (
        bool(raw_sense.get("form_of"))
        or bool(raw_sense.get("alt_of"))
        or bool(tags & {"form-of", "alt-of"})
    )


def idiom_senses_for_item(item: dict):
    """Yield only the ordinary senses that make a multi-word entry an idiom."""
    # A proper name (surname, place, institution) is never an idiom, and its
    # case-folded words would match ordinary text (#2380).
    if str(item.get("pos", "")).casefold() == WIKTIONARY_NAME_POS:
        return
    senses = []
    for raw_sense in item.get("senses", []):
        if not isinstance(raw_sense, dict) or sense_is_form_or_alt_of(item, raw_sense):
            continue
        for upos in upos_for(item, raw_sense):
            sense = sense_from(item, raw_sense, upos)
            if sense is not None:
                senses.append((upos, sense))
    # An entry that only points at the literal reading has no idiomatic sense.
    # Beside a real sense the marker stays, as evidence of literal use (#2378).
    if all(LITERAL_USE_MARKER.match(sense["Gloss"]) for _upos, sense in senses):
        return
    yield from senses


@dataclass(frozen=True)
class ReviewedIdiom:
    headword: str
    words: tuple[str, ...]


@dataclass(frozen=True)
class ReviewedIdioms:
    version: str = DEFAULT_REVIEWED_IDIOM_VERSION
    languages: dict[str, tuple[ReviewedIdiom, ...]] = field(default_factory=dict)


def default_reviewed_idioms_path() -> Path:
    return Path(__file__).resolve().parents[1] / "reviewed_idioms.json"


def _reviewed_idiom_rows(language: str, value: object) -> tuple[ReviewedIdiom, ...]:
    if not isinstance(value, list):
        raise ValueError(f"reviewed idioms for {language!r} must be a list")
    reviewed: list[ReviewedIdiom] = []
    for row in value:
        if not isinstance(row, dict):
            raise ValueError(f"reviewed idiom in {language!r} must be an object")
        headword = row.get("headword")
        words = row.get("words")
        if not isinstance(headword, str) or not headword.strip():
            raise ValueError(f"reviewed idiom in {language!r} needs a headword")
        if not isinstance(words, list) or not all(
            isinstance(word, str) and word.strip() for word in words
        ):
            raise ValueError(f"reviewed idiom {headword!r} needs non-empty words")
        if not IDIOM_MIN_WORDS <= len(words) <= IDIOM_MAX_WORDS:
            raise ValueError(
                f"reviewed idiom {headword!r} must have {IDIOM_MIN_WORDS}-{IDIOM_MAX_WORDS} words"
            )
        normalized_headword = " ".join(unicodedata.normalize("NFC", headword).split())
        normalized_words = tuple(unicodedata.normalize("NFC", word.strip()) for word in words)
        if tuple(normalized_headword.split()) != normalized_words:
            raise ValueError(
                f"reviewed idiom {normalized_headword!r} does not spell its words {normalized_words!r}"
            )
        reviewed.append(ReviewedIdiom(headword=normalized_headword, words=normalized_words))
    return tuple(reviewed)


def load_reviewed_idioms(path: Path) -> ReviewedIdioms:
    """Load Mouseion's reviewed idiom list for every language.

    The list is reviewed data, not dictionary input: malformed rows fail the
    derivation instead of silently disappearing from the index.
    """
    with path.open(encoding="utf-8") as source:
        data = json.load(source)
    if not isinstance(data, dict):
        raise ValueError("reviewed idioms must be an object")
    version = data.get("version")
    if not isinstance(version, str) or not version.strip():
        version = DEFAULT_REVIEWED_IDIOM_VERSION
    languages = data.get("languages", {})
    if not isinstance(languages, dict):
        raise ValueError("reviewed idioms languages must be an object")
    reviewed: dict[str, tuple[ReviewedIdiom, ...]] = {}
    for raw_language, rows in languages.items():
        language = str(raw_language).strip().lower()
        if language not in LANGUAGES:
            raise ValueError(f"reviewed idioms name an unsupported language: {raw_language!r}")
        reviewed[language] = _reviewed_idiom_rows(language, rows)
    return ReviewedIdioms(version=version.strip(), languages=reviewed)


@dataclass
class IdiomAccumulator:
    """The deduplicated idiom entries and their word lookup rows."""

    entries: dict[tuple[str, str, str, str], dict] = field(default_factory=dict)

    def add(  # noqa: PLR0913 - one row's columns, kept explicit for provenance
        self,
        language: str,
        headword: str,
        upos: str,
        words: list[str],
        senses: list[dict],
        source: str,
        version: str,
    ) -> None:
        normalized_words = [normalize(language, word) for word in words]
        if not normalized_words or any(not word for word in normalized_words):
            return
        key = (language, headword, upos, source)
        entry = self.entries.setdefault(
            key,
            {"words": normalized_words, "senses": [], "version": version},
        )
        known = {json.dumps(sense, ensure_ascii=False, sort_keys=True) for sense in entry["senses"]}
        for sense in senses:
            identity = json.dumps(sense, ensure_ascii=False, sort_keys=True)
            if identity not in known:
                known.add(identity)
                entry["senses"].append(sense)

    def rows(self) -> tuple[list[tuple], list[tuple]]:
        entries = []
        words = []
        for (language, headword, upos, source), entry in sorted(self.entries.items()):
            words_json = json.dumps(entry["words"], ensure_ascii=False, separators=(",", ":"))
            senses_json = json.dumps(entry["senses"], ensure_ascii=False, separators=(",", ":"))
            entries.append(
                (language, headword, upos, words_json, senses_json, source, entry["version"])
            )
            for word in dict.fromkeys(entry["words"]):
                words.append((language, word, headword, upos, source))
        return entries, words


def collect_idiom_entries(input_path: Path, provider_version: str) -> IdiomAccumulator:
    """Collect the two-to-four-word dictionary entries that contain a word."""
    accumulator = IdiomAccumulator()
    for item in parsed_items(input_path):
        key = indexed_key(item)
        if key is None:
            continue
        language = key[0]
        word = item.get("word")
        if not isinstance(word, str) or not word.strip():
            continue
        words = idiom_headword_words(word)
        if not IDIOM_MIN_WORDS <= len(words) <= IDIOM_MAX_WORDS:
            continue
        headword = " ".join(words)
        grouped: dict[str, list[dict]] = {}
        for upos, sense in idiom_senses_for_item(item):
            grouped.setdefault(upos, []).append(sense)
        for upos, senses in grouped.items():
            accumulator.add(
                language,
                headword,
                upos,
                words,
                senses,
                WIKTIONARY_IDIOM_SOURCE,
                provider_version,
            )
    return accumulator


def add_reviewed_idioms(accumulator: IdiomAccumulator, reviewed: ReviewedIdioms) -> None:
    """Add the reviewed list's rows under their own source and version."""
    for language, rows in reviewed.languages.items():
        for row in rows:
            accumulator.add(
                language,
                row.headword,
                "",
                list(row.words),
                [],
                REVIEWED_IDIOM_SOURCE,
                reviewed.version,
            )


def french_form_evidence_id(form: str, upos: str, target: str, participle: bool) -> str:
    """Derive stable provenance for one French form-of link."""
    identity = json.dumps(
        {"form": form, "upos": upos, "target": target, "participle": participle},
        ensure_ascii=False,
        sort_keys=True,
    )
    digest = hashlib.sha256(identity.encode("utf-8")).hexdigest()
    return "wiktionary-form-of:" + digest


def french_form_of_targets(raw: dict) -> list[str]:
    """Return the structured form-of headwords a Kaikki sense records.

    Only the explicit ``form_of`` field is evidence. Gloss text is never parsed.
    """
    targets = []
    for target in raw.get("form_of", []):
        word = target.get("word") if isinstance(target, dict) else target
        if isinstance(word, str) and word.strip():
            targets.append(word.strip())
    return targets


def french_sense_links(item: dict, raw_sense: dict) -> tuple[list[str], bool, bool]:
    """Return a French sense's form-of targets and whether it is a form.

    The participle flag comes from Wiktionary's structured tags and is what
    lets a participial adjective match the analyzed verb it belongs to.
    """
    targets = french_form_of_targets(raw_sense)
    tags = tags_for(item, raw_sense)
    is_form_of = bool(targets) or any(tag.casefold() == "form-of" for tag in tags)
    participle = any(tag.casefold() == "participle" for tag in tags)
    return targets, is_form_of, participle


@dataclass
class FrenchFormAccumulator:
    """The French form-of and headword sets collected during one pass."""

    forms: list[tuple[str, str, str, int, str]] = field(default_factory=list)
    headwords: set[tuple[str, str]] = field(default_factory=set)
    seen: set[tuple[str, str, str, bool]] = field(default_factory=set)


def record_french_sense(
    accumulator: FrenchFormAccumulator,
    item: dict,
    form: str,
    raw_sense: object,
) -> None:
    if not isinstance(raw_sense, dict):
        return
    if not [gloss for gloss in values(raw_sense.get("glosses")) if gloss.strip()]:
        return
    targets, is_form_of, participle = french_sense_links(item, raw_sense)
    for upos in upos_for(item, raw_sense):
        if not is_form_of:
            accumulator.headwords.add((form, upos))
        for target in targets:
            normalized_target = normalize(FRENCH_LANGUAGE, target)
            key = (form, upos, normalized_target, participle)
            if not normalized_target or key in accumulator.seen:
                continue
            accumulator.seen.add(key)
            accumulator.forms.append(
                (
                    form,
                    upos,
                    normalized_target,
                    int(participle),
                    french_form_evidence_id(form, upos, normalized_target, participle),
                )
            )


def collect_french_form_data(
    input_path: Path,
) -> tuple[list[tuple[str, str, str, int, str]], list[tuple[str, str]]]:
    """Collect French form-of links and the entries that are real headwords.

    A French entry is a headword only when at least one of its indexed senses is
    not a form-of sense. A form-of-only entry is still in the entries table, but
    it is not offered as a competing surface headword.
    """
    accumulator = FrenchFormAccumulator()
    for item in parsed_items(input_path):
        if item.get("lang_code") != FRENCH_LANGUAGE:
            continue
        word = item.get("word")
        if not isinstance(word, str) or not word.strip():
            continue
        form = normalize(FRENCH_LANGUAGE, word)
        for raw_sense in item.get("senses", []):
            record_french_sense(accumulator, item, form, raw_sense)
    return accumulator.forms, sorted(accumulator.headwords)


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
    for item in parsed_items(input_path):
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
# LSJ keys mark a vowel's quantity with "^" (long) or "_" (short). Quantity is
# not part of the ancient-greek/1 key and OGA lemmas do not carry it, so the
# mark is dropped rather than rendered.
LSJ_LENGTH_MARKS = frozenset({"_", "^"})


def strip_trailing_digits(value: str) -> str:
    """Drop a Beta Code homograph's trailing digits from its key."""
    end = len(value)
    while end > 0 and value[end - 1] in "0123456789":
        end -= 1
    return value[:end]


def strip_length_marks(value: str) -> str:
    """Drop LSJ's "^"/"_" vowel-quantity marks; quantity is not part of the key."""
    return "".join(character for character in value if character not in LSJ_LENGTH_MARKS)


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
    greek, unknown = beta_to_greek(strip_length_marks(key))
    headword = greek.strip()
    return headword, bool(headword) and not unknown


# LSJ marks a real homograph with a letter in parentheses after the headword
# ("a)/n (A)", "a)/n (B)"). Such an entry is a distinct lexeme even when it only
# redirects, so it stays a competing homograph.
LSJ_HOMOGRAPH_MARKERS = tuple(f"({letter})" for letter in "ABCDEFGHIJKLMNOPQRSTUVWXYZ")


def lsj_has_homograph_marker(preamble: str) -> bool:
    """Report whether LSJ marks the entry as a real homograph, "(A)".."(Z)"."""
    return any(marker in preamble for marker in LSJ_HOMOGRAPH_MARKERS)


def lsj_homograph_number(key: str) -> int:
    """Return a key's trailing homograph number, or 1 when it has none."""
    stripped = strip_trailing_digits(key)
    return int(key[len(stripped) :]) if stripped != key else 1


def lsj_first_orth(element) -> str:
    """Return the entry's first <orth>, the headword it spells."""
    for child in element:
        if child.tag == "orth":
            return "".join(child.itertext()).strip()
    return ""


def lsj_preamble(element) -> str:
    """Return the entry's own text before its first sense."""
    parts: list[str] = []
    for child in element:
        if child.tag == "sense":
            break
        parts.append("".join(child.itertext()))
        parts.append(child.tail or "")
    return " ".join(parts)


def lsj_has_gloss(element) -> bool:
    """Report whether the entry states any English gloss of its own."""
    return any(clean_gloss(element_text(tr)) for tr in element.iter("tr"))


def lsj_has_cross_reference(element) -> bool:
    """Report whether the entry carries a cross-reference ("v. ..." / "= ...")."""
    return element.find(".//xr") is not None


def lsj_subordinate(element) -> bool:
    """Report whether a numbered LSJ entry is a sub-entry, not a homograph.

    LSJ files a phrase ("ou) ga/r") or a redirect stub ("h)/dh, v. *ei)/dw")
    under the bare word's key with a trailing homograph number. Neither is the
    bare lemma's own entry, so neither makes it ambiguous when the bare lemma is
    looked up. A real homograph LSJ marks (A)/(B)/(C) still competes.
    """
    if lsj_homograph_number(element.get("key") or "") < 2:
        return False
    if lsj_has_homograph_marker(lsj_preamble(element)):
        return False
    if " " in lsj_first_orth(element):
        return True
    return not lsj_has_gloss(element) and lsj_has_cross_reference(element)


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


# LSJ also marks cognates and loan-words with <tr>: after a language
# abbreviation, usually in the closing "(Cf. Skt. ..., Lat. ...)" etymology.
# Those are not English glosses, so a <tr> after one, or inside a parenthesis
# that names one, is skipped.
#
# LSJ sometimes omits the period after the abbreviations it uses constantly
# ("cf. Lith girdziù", "OHG ei"), so the period is optional for those. The rest
# are written with one, and the abbreviations that are also ordinary words
# ("Copt", "Sumer", "Egypt") only count with it, so prose such as "the entrance
# to Egypt" is not read as a citation. Grammar and citation abbreviations
# (Adv., Subst., Comp., Lit., Rom.) are absent: they introduce English glosses,
# not cognates.
LSJ_COGNATE_LANGUAGES = (
    "Skt",
    "Lat",
    "Lith",
    "Goth",
    "Engl",
    "OE",
    "OHG",
    "Slav",
    "Arm",
    "OIr",
    "Umbr",
    "Osc",
    "Russ",
    "Icel",
    "Av",
    "Hitt",
)
LSJ_COGNATE_LANGUAGE_WORDS = (
    "Heb",
    "Hebr",
    "Aram",
    "Assyr",
    "Bab",
    "Sumer",
    "Accad",
    "Egypt",
    "Copt",
    "Skr",
    "Avest",
    "Lett",
    "OPruss",
    "OSlav",
    "Serb",
    "Celt",
    "Ir",
    "Ital",
    "Armen",
    "Phryg",
)


def lsj_language_pattern(languages: tuple[str, ...]) -> str:
    """Match one LSJ language abbreviation at a word boundary."""
    return r"\b(?:" + "|".join(languages) + r")"


LSJ_COGNATE_LANGUAGE = re.compile(
    lsj_language_pattern(LSJ_COGNATE_LANGUAGES)
    + r"\.?\s*$"
    + "|"
    + lsj_language_pattern(LSJ_COGNATE_LANGUAGE_WORDS)
    + r"\.\s*$"
)
LSJ_COGNATE_LANGUAGE_ANYWHERE = re.compile(
    lsj_language_pattern(LSJ_COGNATE_LANGUAGES + LSJ_COGNATE_LANGUAGE_WORDS) + r"\."
)
LSJ_ETYMOLOGY_NOTE = re.compile(r"\(\s*Cf\.")
LSJ_GLOSS_TRIM = " \t\n,;:"
LSJ_MEANING_START = ":\u2014"


def open_parenthesis(flow: str) -> str:
    """Return the text of the outermost still-open parenthesis, or ''."""
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


@dataclass
class SenseContent:
    """One LSJ sense's own content, gathered in document order. Definitions are
    the glosses LSJ states before the sense's first citation; later glosses
    translate cited usages."""

    glosses: list[str] = field(default_factory=list)
    definitions: list[str] = field(default_factory=list)
    citations: list[str] = field(default_factory=list)
    # flow is the sense's own non-Greek text so far, so each <tr> can be judged
    # by what precedes it.
    flow: list[str] = field(default_factory=list)
    cited: bool = False


def collect_gloss(tr, content: SenseContent) -> None:
    """Record one <tr> as a gloss unless it is a cognate or only punctuation."""
    text = element_text(tr)
    cleaned = "" if is_cognate("".join(content.flow)) else clean_gloss(text)
    if cleaned:
        content.glosses.append(cleaned)
        if not content.cited:
            content.definitions.append(cleaned)
    content.flow.append(text)


def add_flow(content: SenseContent, text: str) -> None:
    """Append sense text. LSJ's ":—" ends a forms paragraph and starts the
    meaning, so the definitions after it are uncited again."""
    content.flow.append(text)
    if LSJ_MEANING_START in text:
        content.cited = False


def collect_sense_content(element, content: SenseContent, track_flow: bool = True) -> None:
    if element.text and track_flow:
        add_flow(content, element.text)
    for child in element:
        # A nested sense is derived on its own; only its tail belongs here.
        if child.tag == "tr":
            collect_gloss(child, content)
        elif child.tag == "bibl":
            content.cited = True
            if citation := lsj_citation(child):
                content.citations.append(citation)
        elif child.tag != "sense":
            # Beta Code writes rough breathing as "(", so Greek text stays out of
            # the flow that parenthesis tracking reads.
            collect_sense_content(child, content, track_flow and child.get("lang") != "greek")
        if child.tail and track_flow:
            add_flow(content, child.tail)


def dedupe(values: list[str]) -> list[str]:
    return list(OrderedDict((value, None) for value in values).keys())


def lsj_sense(element) -> dict:
    content = SenseContent()
    collect_sense_content(element, content)
    sense: dict = {
        "id": (element.get("id") or "").strip(),
        "glosses": dedupe(content.glosses),
        "citations": dedupe(content.citations),
    }
    if content.definitions:
        sense["definitions"] = dedupe(content.definitions)
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


def lsj_rows_for_file(path: Path) -> list[tuple[str, str, str, int]]:
    rows: list[tuple[str, str, str, int]] = []
    # The LSJ files are operator-supplied and digest-pinned (ADR 0101), never
    # fetched from an untrusted source, so entity-expansion attacks are out of
    # scope here.
    for _, element in ElementTree.iterparse(path, events=("end",)):  # noqa: S314
        if element.tag not in ("entryFree", "entry") or element.get("type") != "main":
            continue
        entry_id = (element.get("id") or "").strip()
        headword, admitted = lsj_headword(element.get("key") or "")
        subordinate = lsj_subordinate(element)
        senses = [lsj_sense(child) for child in element if child.tag == "sense"]
        element.clear()
        if not admitted or not entry_id or not senses:
            continue
        senses_json = json.dumps(senses, ensure_ascii=False, separators=(",", ":"))
        rows.append((entry_id, headword, senses_json, int(subordinate)))
    return rows


def collect_lsj_entries(lsj_dir: Path) -> list[tuple[str, str, str, int]]:
    rows: list[tuple[str, str, str, int]] = []
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
    """The row sets a derivation collects before writing the index."""

    entries: dict[tuple[str, str, str], dict]
    greek_entries: list[tuple[str, str, str, str, str]]
    lsj_entries: list[tuple[str, str, str, int]]
    french_forms: list[tuple[str, str, str, int, str]]
    french_headwords: list[tuple[str, str]]
    idiom_entries: list[tuple[str, str, str, str, str, str, str]]
    idiom_words: list[tuple[str, str, str, str, str]]


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
        connection.executemany(INSERT_FRENCH_FORMS_SQL, rows.french_forms)
        connection.executemany(INSERT_FRENCH_HEADWORDS_SQL, rows.french_headwords)
        connection.executemany(INSERT_IDIOM_ENTRIES_SQL, rows.idiom_entries)
        connection.executemany(INSERT_IDIOM_WORDS_SQL, rows.idiom_words)
        connection.commit()
        connection.execute("PRAGMA journal_mode = OFF")
        connection.close()
        temporary_path.chmod(0o644)
        temporary_path.replace(output_path)
    finally:
        temporary_path.unlink(missing_ok=True)


def derive(  # noqa: PLR0913 - the derivation's optional inputs, one per source
    input_path: Path,
    output_path: Path,
    provider_version: str,
    source: SourceMetadata | None = None,
    lsj: LsjSource | None = None,
    reviewed_idioms: ReviewedIdioms | None = None,
) -> None:
    source = source or SourceMetadata()
    if not source.source_sha256:
        source = replace(source, source_sha256=file_sha256(input_path))
    if reviewed_idioms is None:
        reviewed_idioms = load_reviewed_idioms(default_reviewed_idioms_path())
    french_forms, french_headwords = collect_french_form_data(input_path)
    idioms = collect_idiom_entries(input_path, provider_version)
    add_reviewed_idioms(idioms, reviewed_idioms)
    idiom_entries, idiom_words = idioms.rows()
    rows = DerivedRows(
        entries=collect_entries(input_path),
        greek_entries=collect_greek_entries(input_path),
        lsj_entries=collect_lsj_entries(lsj.directory) if lsj else [],
        french_forms=french_forms,
        french_headwords=french_headwords,
        idiom_entries=idiom_entries,
        idiom_words=idiom_words,
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
    parser.add_argument(
        "--reviewed-idioms",
        type=Path,
        help="reviewed idiom list (default: nlp/reviewed_idioms.json)",
    )
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
    try:
        reviewed_idioms = load_reviewed_idioms(
            args.reviewed_idioms or default_reviewed_idioms_path()
        )
    except (OSError, ValueError) as error:
        parser.error(f"could not load reviewed idioms: {error}")
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
        reviewed_idioms,
    )
    print(f"Wrote {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
