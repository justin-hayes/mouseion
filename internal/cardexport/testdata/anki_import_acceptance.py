"""Real Anki-backend import assertions for the optional Go acceptance test."""

import sys
from pathlib import Path

from anki.collection import Collection, ImportAnkiPackageRequest


work = Path(sys.argv[1])
expected_guid = sys.argv[2]

collection = Collection(str(work / "collection.anki2"))


def import_package(filename: str) -> None:
    collection.import_anki_package(
        ImportAnkiPackageRequest(package_path=str(work / filename))
    )


def snapshot():
    notes = collection.db.all("select id,guid,flds,mid from notes")
    cards = collection.db.all("select nid,did from cards")
    decks = {
        name: {"id": deck_id}
        for name, deck_id in collection.db.all("select name,id from decks")
    }
    assert len(notes) == 1, f"expected one note, got {len(notes)}"
    assert len(cards) == 1, f"expected one card, got {len(cards)}"
    return notes[0], cards[0], decks


import_package("book.apkg")
book_note, book_card, decks = snapshot()
assert book_note[1] == expected_guid
book_model = collection.models.get(book_note[3])
assert book_model["name"] == "Mouseion Vocab Recognition"
assert [field["name"] for field in book_model["flds"]] == [
    "Identity",
    "Text",
    "Article",
    "Lemma",
    "Plural",
    "IPA",
    "PrincipalParts",
    "POS",
    "Gloss",
    "English",
    "EnglishSentence",
    "BookTitle",
]
assert "Mouseion\x1fde\x1fBook A" in decks
book_deck_id = decks["Mouseion\x1fde\x1fBook A"]["id"]
assert book_card[1] == book_deck_id

print(
    "Observed with Anki backend: a Book package imported one note with the "
    "expected GUID, model fields, and Book deck placement."
)
collection.close()
