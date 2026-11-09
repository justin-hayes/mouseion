# PROTOTYPE: a work's grouped Books in My Books (#2323)

Throwaway. Not for main. Adding a whole work from the Classical catalog creates one
Inbox Book per constituent book (#2298). This prototype asks how Ancient Greek My
Books shows them as one work row: identity and bucket summary, listing the books and
moving them to To Read without 24 equal buttons, what the filters count, how search
matches, how Hide works, and compact reflow.

```sh
python3 -m http.server -d internal/webapp 8124
# open http://127.0.0.1:8124/prototype-work-books/?variant=A
```

The page loads the app's real `/static/app.css`. Data is illustrative and every
action is in memory; reloading resets it. The starting library is mid-way: Iliad
(Book 1 Read, Book 2 current, Books 3, 4 and 6 To Read with 4 preparing and 6
failing to fetch, Book 9 hidden, the rest Inbox), Anabasis (Book 1 Read, Book 2 To
Read), the Odyssey just added, the Apology (a whole-work Book) and Histories Book 1
only.

- **A, Disclosure + selection**: the books are a table in a "Books" disclosure;
  checkboxes and one "With selected" set (Move to To Read, Move to Inbox, Hide).
  Counts Books.
- **B, Chosen books as rows**: current, To Read and Read Books are nested rows with
  their own margin and actions; Inbox books fold into one line with a
  "Move Book [n] to To Read" picker. Counts Books.
- **C, Work page**: the list has one entry per work and the tabs count entries; the
  books live on a My Books work page with a table and a range move.
- **D, Numbered strip**: one numbered cell per book, shaped by bucket; choosing a
  cell opens that Book's own line with ordinary actions. Counts Books.
- **E, Strip, work-level To Read**: D's strip, but a grouped work moves to To Read
  (and back to Inbox) as a whole; Read books are skipped and the current Book stays
  To Read. No per-book Move or Hide. Interleaving happens on a rough Reading chooser
  (top-bar Reading link), grouped by work with the same strip. Counts Books.

Shared across variants: the margin shows one badge for the work's leading bucket
(same precedence as a Book's bucket) plus counts, and lists any problem with its
recovery action; "More actions" on the work row hides or unhides all its Books;
search matches author, English or Greek title accent-insensitively, and a trailing
number narrows to that book (`iliad 6`); a work with only one Book in My Books stays
an ordinary row ("Histories, Book 1", "Book 1 of 9").

The bottom-right panel has a compact-width (390px) toggle, scenario resets, and the
full state.
