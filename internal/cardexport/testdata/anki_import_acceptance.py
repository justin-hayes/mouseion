"""Real Anki-backend import assertions for the optional Go acceptance test."""

import sys
from pathlib import Path

from anki.collection import Collection, ImportAnkiPackageRequest


work = Path(sys.argv[1])
expected_guid, first_identity, changed_identity, stable_custom_deck = sys.argv[2:]
assert first_identity != changed_identity
stable_custom_deck = stable_custom_deck.replace("::", "\x1f")

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
    assert len(notes) == 1, f"expected one shared note, got {len(notes)}"
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
book_fields = book_note[2]
book_deck_id = decks["Mouseion\x1fde\x1fBook A"]["id"]
assert book_card[1] == book_deck_id

# Anki's real importer reports the shared GUID as a duplicate. With its default
# import behavior it preserves the existing note fields and card placement.
import_package("custom.apkg")
note_after_custom, card_after_custom, decks = snapshot()
assert note_after_custom[1] == expected_guid
assert note_after_custom[2] == book_fields
assert card_after_custom[1] == book_deck_id
assert stable_custom_deck in decks

# Preparing again with changed source evidence and a renamed Custom deck still
# imports into the same stable Anki deck name, while the package description
# carries the updated learner-facing display name.
import_package("renamed-prepare-again.apkg")
note_after_rename, card_after_rename, decks = snapshot()
assert note_after_rename[1] == expected_guid
assert note_after_rename[2] == book_fields
assert card_after_rename[1] == book_deck_id
assert stable_custom_deck in decks
assert sum(name == stable_custom_deck for name in decks) == 1
renamed_anki_deck = collection.decks.get(decks[stable_custom_deck]["id"])
assert renamed_anki_deck["name"] == stable_custom_deck.replace("\x1f", "::")
assert "Custom deck: Renamed" in renamed_anki_deck["desc"]

print(
    "Observed with Anki backend: Book then Custom produced one note; default "
    "imports retained the original note fields and Book deck placement. "
    "Prepare again with changed source sentence and rename retained that note, "
    "reused the same Anki deck, and updated its description with the new title."
)
collection.close()
