# Pico replacement pilot: no-go

**Decision:** Do not replace Pico or start the migration described by #1441.
Keep Pico in production and leave #1442 open for maintainer direction. The
standalone compiler and daisyUI bundle are technically compatible, but the
candidate did not preserve native-control styling or form hierarchy with the
current Mouseion stylesheet. This is a failure of the pilot's product and
maintenance gate, not a browser or build blocker.

## Browser pilot

The fixture server served the real `/library` My Books page and `/catalogs`
connection/feedback page in Chromium at 1280×800 and 375×667, with light and
dark system color schemes. Baseline screenshots use Pico 2.1.1 plus
`app.css`. Candidate screenshots remove Pico and add the compiled Tailwind /
daisyUI stylesheet plus the same `app.css`; a candidate-only primary action was
given `btn btn-primary`. No learner-facing markup or server behavior was
changed. Screenshots are included in [`pico-replacement-pilot/`](pico-replacement-pilot/).

| Screen | Pico baseline | Candidate |
| --- | --- | --- |
| My Books, desktop/light | [baseline](pico-replacement-pilot/books-pico-desktop-light.png) | [candidate](pico-replacement-pilot/books-tailwind-daisyui-desktop-light.png) |
| Catalogs form and feedback, desktop/light | [baseline](pico-replacement-pilot/catalogs-pico-desktop-light.png) | [candidate](pico-replacement-pilot/catalogs-tailwind-daisyui-desktop-light.png) |
| My Books, compact/dark | — | [candidate](pico-replacement-pilot/books-tailwind-daisyui-compact-dark.png) |
| Catalogs form and feedback, compact/dark | — | [candidate](pico-replacement-pilot/catalogs-tailwind-daisyui-compact-dark.png) |

### Observed evidence

- **Book hierarchy:** the candidate retained the bibliographic serif role and
  cover-led reading order from Mouseion's own stylesheet. But the My Books
  layout and action rhythm changed substantially once the current foundation
  was removed; the Book-heavy screen was not an acceptable drop-in replacement.
- **Native forms:** Tailwind Preflight reset the native inputs while the
  existing stylesheet supplied no replacement for Pico's form defaults. On the
  compact Catalogs form, a representative field changed from 309×50px with a
  visible border/background to 207×24px with a transparent background and no
  border. The desktop example changed from 395×63px to 207×24px. Field grouping
  and spacing also collapsed in the screenshot.
- **daisyUI:** the standalone plugin compiled and a deliberately selected
  `btn btn-primary` action could be styled with Mouseion-mapped light and dark
  theme colors. It did not restore the native input treatment or the Catalogs
  form's layout. The candidate still needs a separate Mouseion-owned control
  layer and workflow styling.
- **Targets and focus:** the tested native text fields were 24px high in the
  candidate, below the established 44px touch target. A focused field retained
  only the browser's 1px automatic outline, not the documented Mouseion focus
  token. Existing app CSS also references 17 distinct `--pico-*` variables,
  which would need deliberate replacement or removal.
- **Responsive and themes:** the real pages were rendered in all four
  compact/desktop × light/dark combinations. The screenshots and computed
  dimensions exposed styling regressions in both schemes; compact candidate
  pages did not gain page-level horizontal overflow in this sample. This is
  not a pass for the broader 200%-zoom contract.
- **Semantics and no-JavaScript:** the pilot changed CSS only, so native HTML
  and the server-rendered forms were not structurally replaced. A dedicated
  no-JavaScript interaction run and a full contrast audit were not performed;
  they remain necessary before any future go decision.

## Build compatibility

The official standalone distributions compiled together without Node:

- Tailwind CSS CLI **v4.3.3**, Linux x64, SHA-256
  `dc61b3ac6b8c9ca874c0cc4c57b2409791a64c5540404ca5f5367360babc313a`
- daisyUI **v5.7.47** standalone plugin, SHA-256
  `85819d3fe86a852237b439b13f481481aac58e562ac47694cd11c3038e994f2a`
- daisyUI standalone theme plugin, SHA-256
  `d92d2f488933dab757f046e81ebd4986e51e9423307798dc3545d09a15542c89`

Compilation succeeded in about 0.1 seconds and emitted a 62,879-byte stylesheet.
The official standalone workflow therefore presents no immediate compiler or
browser compatibility blocker for this Chromium pilot. This was a throwaway
build outside the repository, not a proposed production asset or reproducibility
gate.

## Recommendation

**No-go for the current candidate as a foundation replacement.** Preserve the
accepted native-control, focus, target-size, theme, and Book-centered contracts
before proposing a full migration. The pilot required more than simply adding
Tailwind/daisyUI beside the existing 1,847-line Mouseion stylesheet: native
controls need a new owner, Pico token references need retirement, and the form
and book workflows need a deliberate port. Do not ship that as a half-migration.

This is evidence for #1442 only. It does not reject Tailwind or authorize
downstream implementation; maintainer direction is needed before revising the
candidate or opening follow-up work.
