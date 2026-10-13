// Optional viewport fitting only. Words and morphology are static, not interactions.
let request;
async function updateWorkspace(url) {
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
    history.replaceState(null, '', url);
  } catch (error) {
    if (error.name === 'AbortError') return;
    // Width failures retain the existing server-rendered diagram and manual controls.
  }
}
function fitWidth() {
  const url = new URL(location.href);
  const diagram = document.querySelector('.diagram-scroll');
  if (!diagram || url.searchParams.get('manual_width') === '1') return;
  const width = Math.max(240, Math.min(1800, diagram.clientWidth));
  if (url.searchParams.get('width') === String(width)) return;
  url.searchParams.set('width', width);
  updateWorkspace(url.href);
}
let resizeTimer;
window.addEventListener('resize', () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(fitWidth, 100); });
document.addEventListener('DOMContentLoaded', fitWidth);
window.addEventListener('popstate', () => location.reload());
document.addEventListener('keydown', event => {
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || event.target.closest('input, textarea, select, [contenteditable], .diagram-scroll')) return;
  const selector = event.key === 'ArrowLeft' ? '.variant-previous' : event.key === 'ArrowRight' ? '.variant-next' : null;
  if (selector) { event.preventDefault(); document.querySelector(selector).click(); }
});
