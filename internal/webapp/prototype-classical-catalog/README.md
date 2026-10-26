# PROTOTYPE: Classical catalog placement (#2298)

Throwaway. Not for main. This prototype asks where the Classical catalog lives
and how a learner browses many works (author, then work, then Reading units),
adds a unit to My Books, and follows its preparation.

```sh
python3 -m http.server -d internal/webapp 8124
# open http://127.0.0.1:8124/prototype-classical-catalog/?variant=A
```

The page loads the app's real `/static/app.css` and copies the shell markup from
the fixture server. Data is illustrative, and every action is in memory.
Reloading resets it.

- **A, In My Books**: a "Classical catalog" view beside "Your Books" in the
  Ancient Greek partition of My Books. Authors, works and units are nested
  disclosures. From German My Books, a link switches to Ancient Greek.
- **B, On Catalogs**: a built-in "Classical catalog" record above the OPDS
  connections. Browsing goes through drill-down pages (author index, then
  author, then work) with breadcrumbs, and there is an "Add all remaining
  books" confirmation.
- **C, Own destination**: a fifth top-bar destination. An author index sits
  beside per-work unit tables that show word counts, and the "In My Books"
  column also tracks preparation.

The variants share the follow-through: Add creates an Inbox Book, and Move to
To Read runs simulated preparation. My Books margin notes follow #2296. The
bottom-right panel sets the outcome (success, fetch unavailable, import
failure or pin mismatch), turns on the legacy-rows fail-closed state, steps
preparation, and prints the full state.
