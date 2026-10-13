// Optional comparison conveniences. Everything else is native, server-rendered HTML.
const support = document.querySelector('.support');
const context = document.querySelector('.context');
document.addEventListener('click', event => {
  const link = event.target.closest('.prototype-switcher a, .word-strip a, .relationship-panel a');
  if (!link) return;
  const url = new URL(link.href);
  url.searchParams.set('support', support.open ? '1' : '0');
  url.searchParams.set('context', context.open ? '1' : '0');
  link.href = url;
});
context.open = new URLSearchParams(location.search).get('context') === '1';
document.addEventListener('keydown', event => {
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || event.target.closest('input, textarea, select, [contenteditable], .diagram-scroll')) return;
  const selector = event.key === 'ArrowLeft' ? '.variant-previous' : event.key === 'ArrowRight' ? '.variant-next' : null;
  if (selector) { event.preventDefault(); document.querySelector(selector).click(); }
});
support.addEventListener('toggle', () => {
  const url = new URL(location.href);
  url.searchParams.set('support', support.open ? '1' : '0');
  history.replaceState(null, '', url);
});
