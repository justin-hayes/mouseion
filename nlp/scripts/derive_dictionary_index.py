#!/usr/bin/env python3
"""Derive Mouseion's compact lexical index from a Kaikki JSONL dump."""

from __future__ import annotations

import argparse
import gzip
import json
import os
import re
import shutil
import sqlite3
import tempfile
import unicodedata
from collections import OrderedDict
from dataclasses import dataclass
from datetime import date
from pathlib import Path
from urllib.request import urlopen

LANGUAGES = {"de", "it", "el"}
# Ancient Greek entries come from English Wiktionary too, but they are stored
# differently: one row per source entry, with the NFC source headword and no
# normalization key. Go derives the ancient-greek/1 key when it opens the
# index. The index format version tells an older index apart so it can report
# grc as unavailable.
ANCIENT_GREEK_LANGUAGE = "grc"
INDEX_FORMAT_VERSION = 2
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
    PRIMARY KEY (language, lemma, upos)
);
CREATE TABLE greek_entries (
    headword TEXT NOT NULL,
    upos TEXT NOT NULL,
    etymology_number TEXT NOT NULL,
    senses_json TEXT NOT NULL,
    forms_json TEXT NOT NULL
);
"""
INSERT_ENTRIES_SQL = (
    "INSERT INTO entries"
    "(language, lemma, upos, senses_json, gender, article, plural, ipa, principal_parts) "
    "VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"
)
INSERT_GREEK_ENTRIES_SQL = (
    "INSERT INTO greek_entries(headword, upos, etymology_number, senses_json, forms_json) "
    "VALUES (?, ?, ?, ?, ?)"
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
    return " ".join(unicodedata.normalize("NFC", value.strip()).casefold().split())


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


def article_for(language: str, word: str, gender: str, forms: object) -> str:
    supplied = supplied_article_for(language, forms)
    if supplied:
        return supplied
    if language == "de":
        return {"Masc": "der", "Fem": "die", "Neut": "das"}.get(gender, "")
    if language == "el":
        return {"Masc": "ο", "Fem": "η", "Neut": "το"}.get(gender, "")
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


def ipa_for(item: dict) -> str:
    best = ""
    best_rank = None
    for sound in item.get("sounds", []):
        if not isinstance(sound, dict):
            continue
        raw = sound.get("ipa")
        if not isinstance(raw, str) or not raw.strip():
            continue
        if sound.get("note"):
            continue
        tags = {tag.casefold() for tag in values(sound.get("tags"))}
        if not tags.issubset(IPA_ACCEPTED_TAGS):
            continue
        candidate = OPTIONAL_SEGMENT_PATTERN.sub("", raw.strip()).strip()
        if not candidate:
            continue
        rank = 0 if candidate.startswith("/") and candidate.endswith("/") else 1
        if best_rank is None or rank < best_rank:
            best = candidate
            best_rank = rank
    return best


def principal_parts_for(language: str, upos: str, item: dict) -> str:
    if language == "el" or upos != "VERB":
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
    return {
        "Gloss": glosses[0].strip(),
        "Examples": examples,
        "Topics": values(raw.get("topics")),
        "Tags": tags,
        "Phrase": phrase.strip(),
        "Gender": gender_for(item, raw),
        "Article": article_for(
            item["lang_code"], item["word"], gender_for(item, raw), item.get("forms")
        )
        if upos == "NOUN"
        else "",
        "Plural": plural_for(item) if upos == "NOUN" else "",
        "IPA": ipa_for(item),
    }


def open_input(path: Path):
    if path.suffix == ".gz":
        return gzip.open(path, mode="rt", encoding="utf-8")
    return path.open(encoding="utf-8")


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
    entry["ipa"] = entry["ipa"] or ipa_for(item)
    entry["principal_parts"] = entry["principal_parts"] or principal_parts_for(language, upos, item)


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


@dataclass(frozen=True)
class SourceMetadata:
    """The dump provenance recorded in the index's metadata table."""

    dump_date: str = ""
    extraction_date: str = ""
    wiktextract_commit: str = ""


def metadata_rows(provider_version: str, source: SourceMetadata) -> dict[str, str]:
    return {
        "index_format": str(INDEX_FORMAT_VERSION),
        "provider_version": provider_version,
        "license": "Wiktionary-derived data: CC BY-SA 4.0 / GFDL",
        "generated_at": date.today().isoformat(),
        "source": "Kaikki.org Wiktextract enwiktionary JSONL",
        "attribution": "Wiktionary contributors; CC BY-SA 4.0 / GFDL",
        "dump_date": source.dump_date,
        "extraction_date": source.extraction_date,
        "wiktextract_commit": source.wiktextract_commit,
    }


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
        )


def write_index(
    output_path: Path,
    entries: dict[tuple[str, str, str], dict],
    greek_entries: list[tuple[str, str, str, str, str]],
    provider_version: str,
    source: SourceMetadata,
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
            metadata_rows(provider_version, source).items(),
        )
        connection.executemany(INSERT_ENTRIES_SQL, entry_rows(entries))
        connection.executemany(INSERT_GREEK_ENTRIES_SQL, greek_entries)
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
) -> None:
    write_index(
        output_path,
        collect_entries(input_path),
        collect_greek_entries(input_path),
        provider_version,
        source or SourceMetadata(),
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
    args = parser.parse_args(argv)
    if args.force_download and not args.download:
        parser.error("--force-download requires --download")
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
    )
    print(f"Wrote {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
