import { LitElement, html, nothing, type TemplateResult } from 'lit';

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
  private occurrences: Occurrence[] | null = null;

  override connectedCallback(): void {
    super.connectedCallback();
    try {
      const host = this.parentElement;
      const native = document.getElementById('concordance-native-results');
      if (!host || !native) return;
       const fragmentTarget = location.hash ? document.getElementById(location.hash.slice(1)) : null;
       const restoreFocusID = location.hash
         ? fragmentTarget && native.contains(fragmentTarget) ? fragmentTarget.id : 'concordance-summary'
         : '';
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
        if (restoreFocusID) document.getElementById(restoreFocusID)?.focus({ preventScroll: true });
      });
    } catch {
      // Enhancement is opportunistic. Invalid data leaves the native result list visible.
    }
  }

  protected override createRenderRoot(): HTMLElement {
    return this;
  }

  override render(): TemplateResult | typeof nothing {
    if (!this.occurrences) return nothing;
    return html`<ol class="concordance-results">
      ${this.occurrences.map((row) => html`<li class="concordance-result">
        <details class="concordance-row" id=${row.id} tabindex="-1" @keydown=${this.onRowKeyDown}>
          <summary aria-label=${`Occurrence of ${row.surface} in ${row.bookTitle}`} @click=${this.onRowClick}>
            <span class="concordance-book-title">${row.bookTitle}</span>
            <span class="concordance-before">${row.left}</span>
            <strong class="concordance-surface">${row.surface}</strong>
            <span class="concordance-after">${row.right}</span>
          </summary>
          <div class="concordance-context"><p class="reading-text">${this.sentence(row)}</p></div>
        </details>
        <a class="concordance-study-link" aria-label="Study this sentence and its syntax" href=${row.studyUrl}>
          <span aria-hidden="true">↗</span><span class="concordance-study-label">Study</span>
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
    this.querySelectorAll<HTMLDetailsElement>('details.concordance-row[open]').forEach((row) => {
      if (row !== opening) row.open = false;
    });
  };

  private readonly onRowKeyDown = (event: KeyboardEvent): void => {
    // Experimental island-only trial for #1379. The accepted feature contract
    // keeps Up/Down as normal page scrolling; this focused-summary behavior is
    // intentionally not applied to the native fallback or other controls.
    if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
    const summary = event.target;
    if (!(summary instanceof HTMLElement) || summary.tagName !== 'SUMMARY' ||
        !summary.matches(':focus')) return;

    const rows = Array.from(this.querySelectorAll<HTMLDetailsElement>('details.concordance-row'));
    const current = summary.closest<HTMLDetailsElement>('details.concordance-row');
    const index = current ? rows.indexOf(current) : -1;
    if (index < 0) return;

    if (event.key === 'ArrowDown' && index < rows.length - 1) {
      event.preventDefault();
      rows[index + 1].querySelector('summary')?.focus();
    } else if (event.key === 'ArrowUp' && index > 0) {
      event.preventDefault();
      rows[index - 1].querySelector('summary')?.focus();
    } else if (event.key === 'Escape' && current?.open) {
      event.preventDefault();
      current.open = false;
      summary.focus();
    }
  };
}

if (!customElements.get('mouseion-concordance')) {
  customElements.define('mouseion-concordance', MouseionConcordance);
}
