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
let request;
async function updateWorkspace(url, replaceHistory, focusInspector) {
  request?.abort();
  const controller = new AbortController();
  request = controller;
  const scroll = document.querySelector('.diagram-scroll');
  const saved = {left: scroll?.scrollLeft || 0, top: scroll?.scrollTop || 0};
  try {
    const response = await fetch(url, {signal: controller.signal});
    if (!response.ok) throw new Error('Prototype request failed');
    const next = new DOMParser().parseFromString(await response.text(), 'text/html');
    if (controller.signal.aborted) return;
    document.querySelector('.chart-workspace').replaceWith(next.querySelector('.chart-workspace'));
    document.querySelector('.prototype-switcher').replaceWith(next.querySelector('.prototype-switcher'));
    const currentScroll = document.querySelector('.diagram-scroll');
    if (currentScroll) { currentScroll.scrollLeft = saved.left; currentScroll.scrollTop = saved.top; }
    if (replaceHistory) history.replaceState(null, '', url);
    else history.pushState(null, '', url);
    const inspector = document.querySelector('.node-inspector');
    if (inspector) {
      if (focusInspector) inspector.focus({preventScroll: true});
      positionInspector();
    } else if (focusInspector) document.querySelector('.chart-workspace').focus({preventScroll: true});
  } catch (error) {
    if (error.name === 'AbortError') return;
    if (!replaceHistory) location.href = url; // Native link fallback; width failures retain the existing diagram.
  }
}
document.addEventListener('click', event => {
  const link = event.target.closest('.node-link, .feature-word, .node-inspector a, .text-alternative a');
  if (!link || event.button !== 0 || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
  event.preventDefault();
  updateWorkspace(link.href, false, true);
});
document.addEventListener('toggle', event => {
  if (event.target.closest('.node-inspector')) positionInspector();
}, true);
window.addEventListener('resize', positionInspector);
window.addEventListener('scroll', positionInspector, true);
function fitWidth() {
  const url = new URL(location.href);
  const variant = document.querySelector('.ss-prototype').dataset.variant;
  const diagram = document.querySelector('.diagram-scroll');
  if (variant === 'tree' || !diagram || url.searchParams.get('manual_width') === '1') return;
  const width = Math.max(240, Math.min(1800, diagram.clientWidth));
  if (url.searchParams.get('width') === String(width)) return;
  url.searchParams.set('width', width);
  updateWorkspace(url.href, true, false);
}
let resizeTimer;
window.addEventListener('resize', () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(fitWidth, 100); });
document.addEventListener('DOMContentLoaded', fitWidth);
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
