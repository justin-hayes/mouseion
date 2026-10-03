import { LitElement, css, html, nothing, type TemplateResult } from 'lit';

type Occurrence = {
  id: string;
  bookTitle: string;
  left: string;
  surface: string;
  right: string;
  sentence: string;
  targetStart: number;
  targetEnd: number;
  studyUrl: string;
};

type Payload = { occurrences: Occurrence[] };

function validOccurrence(value: unknown): value is Occurrence {
  if (!value || typeof value !== 'object') return false;
  const row = value as Partial<Occurrence>;
  return typeof row.id === 'string' && /^occurrence-[\w-]+-\d+-\d+$/.test(row.id) &&
    typeof row.bookTitle === 'string' && row.bookTitle.length > 0 &&
    typeof row.left === 'string' && typeof row.surface === 'string' && row.surface.length > 0 &&
    typeof row.right === 'string' && typeof row.sentence === 'string' &&
    Number.isInteger(row.targetStart) && Number.isInteger(row.targetEnd) &&
    row.targetStart! >= 0 && row.targetEnd! > row.targetStart! && row.targetEnd! <= row.sentence.length &&
    row.sentence.slice(row.targetStart, row.targetEnd) === row.surface &&
    typeof row.studyUrl === 'string' && row.studyUrl.startsWith('/vocabulary/concordance/sentence?');
}

class MouseionConcordance extends LitElement {
  static override styles = css`
    :host {
      display: block;
      color: var(--mouseion-color-text, inherit);
      font-family: var(--mouseion-font-application, sans-serif);
    }
    .concordance-results {
      list-style: none;
      padding-inline-start: 0;
      margin-block: var(--mouseion-space-3, 0.75rem);
    }
    .concordance-result {
      display: grid;
      grid-template-columns: minmax(0, 1fr) auto;
      align-items: stretch;
      gap: var(--mouseion-space-2, 0.5rem);
      border-bottom: 1px solid var(--mouseion-color-border, currentColor);
    }
    .concordance-row {
      min-width: 0;
      margin: 0;
      border: 0;
      border-radius: 0;
      padding-block: var(--mouseion-space-2, 0.5rem);
    }
    .concordance-row summary {
      display: grid;
      grid-template-columns: minmax(9rem, 0.8fr) minmax(0, 1fr) max-content minmax(0, 1fr);
      align-items: baseline;
      gap: var(--mouseion-space-2, 0.5rem);
      line-height: 1.5;
      max-width: 100%;
      cursor: pointer;
    }
    .concordance-book-title {
      min-width: 0;
      font-family: var(--mouseion-font-reading, serif);
      font-weight: 600;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .concordance-before,
    .concordance-after {
      min-width: 0;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .concordance-surface,
    .concordance-observed-target {
      color: var(--mouseion-color-accent, currentColor);
      white-space: nowrap;
    }
    .concordance-study-link {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: var(--mouseion-space-1, 0.25rem);
      min-width: 2.75rem;
      padding-inline: var(--mouseion-space-2, 0.5rem);
      color: var(--mouseion-color-accent, currentColor);
      text-align: center;
    }
    .concordance-context {
      max-width: var(--mouseion-width-reading, 42rem);
      padding: var(--mouseion-space-2, 0.5rem) var(--mouseion-space-3, 0.75rem) 0;
    }
    .concordance-context p {
      overflow-wrap: anywhere;
      margin-block: 0 var(--mouseion-space-2, 0.5rem);
      font-family: var(--mouseion-font-reading, serif);
      line-height: 1.7;
    }
    :focus-visible {
      outline: 2px solid var(--mouseion-color-focus, currentColor);
      outline-offset: 2px;
    }
    @media (max-width: 40rem) {
      .concordance-result { grid-template-columns: minmax(0, 1fr) auto; }
      .concordance-row summary {
        grid-template-columns: minmax(0, 1fr) max-content;
        gap: var(--mouseion-space-1, 0.25rem) var(--mouseion-space-2, 0.5rem);
      }
      .concordance-book-title {
        grid-column: 1 / -1;
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
      }
      .concordance-before { grid-column: 1; white-space: normal; }
      .concordance-surface { grid-column: 2; grid-row: 2; }
      .concordance-after { grid-column: 1 / -1; white-space: normal; }
      .concordance-study-link {
        align-self: start;
        min-width: 0;
        padding-inline: var(--mouseion-space-1, 0.25rem);
      }
    }
  `;

  private occurrences: Occurrence[] | null = null;

  override connectedCallback(): void {
    super.connectedCallback();
    window.addEventListener('hashchange', this.restoreFragmentFocus);
    window.addEventListener('popstate', this.restoreFragmentFocus);
    try {
      const host = this.parentElement;
      const native = document.getElementById('concordance-native-results');
      if (!host || !native) return;
      const payload: unknown = JSON.parse(host.getAttribute('data-concordance-data') ?? '');
      if (!payload || typeof payload !== 'object' ||
          !Array.isArray((payload as Payload).occurrences) ||
          !(payload as Payload).occurrences.every(validOccurrence) ||
          (payload as Payload).occurrences.length === 0) return;
      this.occurrences = (payload as Payload).occurrences;
      this.requestUpdate();
      void this.updateComplete.then(() => {
        if (!this.isConnected || !this.occurrences) return;
        native.remove();
        host.hidden = false;
        this.restoreFragmentFocus();
      });
    } catch {
      // Enhancement is opportunistic. Invalid data leaves the native result list visible.
    }
  }

  override disconnectedCallback(): void {
    window.removeEventListener('hashchange', this.restoreFragmentFocus);
    window.removeEventListener('popstate', this.restoreFragmentFocus);
    super.disconnectedCallback();
  }

  private readonly restoreFragmentFocus = (): void => {
    if (!this.isConnected || !this.occurrences || !location.hash) return;
    const id = location.hash.slice(1);
    const target = this.shadowRoot?.getElementById(id);
    (target ?? document.getElementById('concordance-summary'))?.focus({ preventScroll: true });
  };

  override render(): TemplateResult | typeof nothing {
    if (!this.occurrences) return nothing;
    return html`<ol class="concordance-results">
      ${this.occurrences.map((row) => html`<li class="concordance-result">
        <details class="concordance-row" id=${row.id} tabindex="-1">
          <summary aria-label=${`Occurrence of ${row.surface} in ${row.bookTitle}`} @click=${this.onRowClick}>
            <span class="concordance-book-title">${row.bookTitle}</span>
            <span class="concordance-before">${row.left}</span>
            <strong class="concordance-surface">${row.surface}</strong>
            <span class="concordance-after">${row.right}</span>
          </summary>
          <div class="concordance-context"><p class="reading-text">${this.sentence(row)}</p></div>
        </details>
        <a class="concordance-study-link" aria-label="Study this sentence and its syntax" title="Study this sentence and its syntax" href=${row.studyUrl}>
          <span aria-hidden="true">↗</span>
        </a>
      </li>`)}
    </ol>`;
  }

  private sentence(row: Occurrence): TemplateResult {
    return html`${row.sentence.slice(0, row.targetStart)}<strong class="concordance-observed-target">${row.surface}</strong>${row.sentence.slice(row.targetEnd)}`;
  }

  private readonly onRowClick = (event: MouseEvent): void => {
    const summary = event.currentTarget;
    if (!(summary instanceof HTMLElement) || summary.tagName !== 'SUMMARY') return;
    const opening = summary.closest<HTMLDetailsElement>('details.concordance-row');
    if (!opening || opening.open) return;
    // Close synchronously before the browser's default summary activation. A
    // toggle event is task-queued and can race rapid successive openings.
    this.shadowRoot?.querySelectorAll<HTMLDetailsElement>('details.concordance-row[open]').forEach((row) => {
      if (row !== opening) row.open = false;
    });
  };

}

if (!customElements.get('mouseion-concordance')) {
  customElements.define('mouseion-concordance', MouseionConcordance);
}
