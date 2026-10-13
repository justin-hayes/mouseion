// Optional conveniences only: all nodes, evidence, dismissals and variants are native links.
function positionInspector() {
  const inspector = document.querySelector('.node-inspector');
  const node = document.querySelector('.selected-node');
  if (!inspector || !node) return;
  const rect = node.getBoundingClientRect();
  const panel = inspector.getBoundingClientRect();
  let left = rect.right + 12;
  if (left + panel.width > innerWidth - 16) left = rect.left - panel.width - 12;
  inspector.style.left = Math.max(16, Math.min(left, innerWidth - panel.width - 16)) + 'px';
  inspector.style.right = 'auto';
  inspector.style.bottom = 'auto';
  inspector.style.top = Math.max(16, Math.min(rect.top, innerHeight - panel.height - 80)) + 'px';
}
const initialInspector = document.querySelector('.node-inspector');
if (initialInspector && location.hash === '#inspector') {
  document.querySelector('.selected-node')?.scrollIntoView({block: 'center', inline: 'nearest'});
  initialInspector.focus({preventScroll: true});
  positionInspector();
}
document.addEventListener('click', async event => {
  const link = event.target.closest('.node-link, .node-inspector a, .text-alternative a');
  if (!link || event.button !== 0 || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
  event.preventDefault();
  const url = link.href;
  const scroll = document.querySelector('.diagram-scroll');
  const saved = {left: scroll?.scrollLeft || 0, top: scroll?.scrollTop || 0};
  try {
    const response = await fetch(url);
    if (!response.ok) throw new Error('Prototype request failed');
    const next = new DOMParser().parseFromString(await response.text(), 'text/html');
    document.querySelector('.chart-workspace').replaceWith(next.querySelector('.chart-workspace'));
    document.querySelector('.prototype-switcher').replaceWith(next.querySelector('.prototype-switcher'));
    const currentScroll = document.querySelector('.diagram-scroll');
    if (currentScroll) { currentScroll.scrollLeft = saved.left; currentScroll.scrollTop = saved.top; }
    history.pushState(null, '', url);
    const inspector = document.querySelector('.node-inspector');
    if (inspector) { inspector.focus({preventScroll: true}); positionInspector(); }
    else document.querySelector('.chart-workspace').focus({preventScroll: true});
  } catch {
    location.href = url; // The ordinary server-rendered path remains the fallback.
  }
});
document.addEventListener('toggle', event => {
  if (event.target.closest('.node-inspector')) positionInspector();
}, true);
window.addEventListener('resize', positionInspector);
window.addEventListener('scroll', positionInspector, true);
window.addEventListener('popstate', () => location.reload());
document.addEventListener('keydown', event => {
  if (event.key === 'Escape' && document.querySelector('.node-inspector')) {
    event.preventDefault();
    document.querySelector('.dismiss-inspector').click();
    return;
  }
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || event.target.closest('input, textarea, select, [contenteditable], .diagram-scroll, .node-inspector')) return;
  const selector = event.key === 'ArrowLeft' ? '.variant-previous' : event.key === 'ArrowRight' ? '.variant-next' : null;
  if (selector) { event.preventDefault(); document.querySelector(selector).click(); }
});
