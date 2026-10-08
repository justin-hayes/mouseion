// THROWAWAY switcher only. Every product-looking interaction is native HTML.
document.addEventListener('keydown', function (event) {
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;
  if (event.target.closest('input, textarea, select, [contenteditable], summary, button')) return;
  var selector = event.key === 'ArrowLeft' ? '[data-cp-previous]' : event.key === 'ArrowRight' ? '[data-cp-next]' : null;
  var link = selector && document.querySelector(selector);
  if (link) {
    event.preventDefault();
    window.location.assign(link.href);
  }
});
