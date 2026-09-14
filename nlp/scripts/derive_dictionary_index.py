#!/usr/bin/env python3
"""Derive Mouseion's compact lexical index from a Kaikki JSONL dump."""

from __future__ import annotations

import argparse
from collections import OrderedDict
from datetime import date
import gzip
import json
import os
from pathlib import Path
import shutil
import sqlite3
import tempfile
import unicodedata
from urllib.request import urlopen


LANGUAGES = {"de", "it"}
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
    "conj": "CCONJ",
    "conjunction": "CCONJ",
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


def load_german_normalization_policy() -> dict:
    path = Path(__file__).resolve().parents[2] / "internal" / "canonicalization" / "german_post1996.json"
    with path.open(encoding="utf-8") as source:
        return json.load(source)


GERMAN_NORMALIZATION_POLICY = load_german_normalization_policy()


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
        return GERMAN_NORMALIZATION_POLICY["equivalences"].get(value, value)
    return " ".join(value.strip().casefold().split())


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
    candidates = tags_for(item, sense)
    for value in candidates:
        if value.casefold() in GENDERS:
            return GENDERS[value.casefold()]
    for category in values(item.get("categories")):
        lowered = category.casefold()
        for name, gender in GENDERS.items():
            if name in lowered:
                return gender
    return ""


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
    for form in forms if isinstance(forms, list) else []:
        if not isinstance(form, dict):
            continue
        tags = {tag.casefold() for tag in values(form.get("tags"))}
        if tags.intersection({"article", "definite", "definite article"}):
            value = form.get("form")
            if isinstance(value, str) and value.strip():
                return value.strip()
    if language == "de":
        return {"Masc": "der", "Fem": "die", "Neut": "das"}.get(gender, "")
    if language != "it":
        return ""
    word = word.casefold().strip()
    if not word or not gender:
        return ""
    if word[0] in "aeiouàèéìòóù":
        return "l'"
    if gender == "Fem":
        return "la"
    if word.startswith(("z", "x", "y", "gn", "ps", "pn")) or (word.startswith("s") and len(word) > 1 and word[1] not in "aeiouàèéìòóù"):
        return "lo"
    return "il"


def ipa_for(item: dict) -> str:
    for sound in item.get("sounds", []):
        if isinstance(sound, dict) and isinstance(sound.get("ipa"), str) and sound["ipa"].strip():
            return sound["ipa"].strip()
    return ""


def sense_from(item: dict, raw: dict) -> dict | None:
    glosses = values(raw.get("glosses"))
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
        "Article": article_for(item["lang_code"], item["word"], gender_for(item, raw), item.get("forms")),
        "Plural": plural_for(item),
        "IPA": ipa_for(item),
    }


def open_input(path: Path):
    if path.suffix == ".gz":
        return gzip.open(path, mode="rt", encoding="utf-8")
    return path.open(encoding="utf-8")


def download_input(force: bool) -> Path:
    try:
        from kaikki_json import config
    except ImportError as error:
        raise RuntimeError(f"--download requires kaikki-json: {error}") from error
    output = Path(config.gz_file_path)
    if output.exists() and not force:
        return output
    output.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(dir=output.parent, prefix=output.name + ".", suffix=".tmp")
    temporary_path = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "wb") as destination, urlopen(KAIKKI_DOWNLOAD_URL) as source:
            shutil.copyfileobj(source, destination)
        temporary_path.replace(output)
    finally:
        temporary_path.unlink(missing_ok=True)
    return output


def derive(input_path: Path, output_path: Path, provider_version: str, dump_date: str = "", extraction_date: str = "", wiktextract_commit: str = "") -> None:
    entries: dict[tuple[str, str, str], dict] = {}
    with open_input(input_path) as source:
        for line in source:
            if not line.strip():
                continue
            item = json.loads(line)
            language = item.get("lang_code")
            if language not in LANGUAGES:
                continue
            upos = POS.get(str(item.get("pos", "")).casefold())
            word = item.get("word")
            if not upos or not isinstance(word, str) or not word.strip():
                continue
            lemma = normalize(language, word)
            key = (language, lemma, upos)
            entry = entries.setdefault(key, {"senses": [], "gender": "", "article": "", "plural": "", "ipa": ""})
            for raw_sense in item.get("senses", []):
                if not isinstance(raw_sense, dict):
                    continue
                sense = sense_from(item, raw_sense)
                if sense is None:
                    continue
                identity = json.dumps(sense, ensure_ascii=False, sort_keys=True)
                if not any(json.dumps(existing, ensure_ascii=False, sort_keys=True) == identity for existing in entry["senses"]):
                    entry["senses"].append(sense)
                entry["gender"] = entry["gender"] or sense["Gender"]
                entry["article"] = entry["article"] or sense["Article"]
                entry["plural"] = entry["plural"] or sense["Plural"]
            entry["ipa"] = entry["ipa"] or ipa_for(item)

    output_path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=output_path.parent, prefix=output_path.name + ".", suffix=".tmp", delete=False) as temporary:
        temporary_path = Path(temporary.name)
    try:
        connection = sqlite3.connect(temporary_path)
        connection.executescript(
            """
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
                PRIMARY KEY (language, lemma, upos)
            );
            """
        )
        metadata = {
            "provider_version": provider_version,
            "license": "Wiktionary-derived data: CC BY-SA 3.0 / GFDL",
            "generated_at": date.today().isoformat(),
            "source": "Kaikki.org Wiktextract enwiktionary JSONL",
            "attribution": "Wiktionary contributors; CC BY-SA 3.0 / GFDL",
            "dump_date": dump_date,
            "extraction_date": extraction_date,
            "wiktextract_commit": wiktextract_commit,
        }
        connection.executemany("INSERT INTO metadata(key, value) VALUES (?, ?)", metadata.items())
        connection.executemany(
            "INSERT INTO entries(language, lemma, upos, senses_json, gender, article, plural, ipa) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
            (
                (language, lemma, upos, json.dumps(entry["senses"], ensure_ascii=False, separators=(",", ":")), entry["gender"], entry["article"], entry["plural"], entry["ipa"])
                for (language, lemma, upos), entry in sorted(entries.items())
            ),
        )
        connection.commit()
        connection.execute("PRAGMA journal_mode = OFF")
        connection.close()
        temporary_path.chmod(0o644)
        temporary_path.replace(output_path)
    finally:
        temporary_path.unlink(missing_ok=True)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--input", type=Path, help="raw Kaikki/Wiktextract JSONL(.gz)")
    source.add_argument("--download", action="store_true", help="download the raw Kaikki dump first")
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
    provider_version = args.provider_version or f"dump={args.dump_date or 'unknown'};extraction={args.extraction_date};wiktextract={args.wiktextract_commit or 'unknown'}"
    derive(input_path, args.output, provider_version, args.dump_date, args.extraction_date, args.wiktextract_commit)
    print(f"Wrote {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
