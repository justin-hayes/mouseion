document.addEventListener('keydown', function (event) {
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return;

  var summary = event.composedPath().find(function (node) {
    return node instanceof HTMLElement && node.tagName === 'SUMMARY';
  });
  if (!(summary instanceof HTMLElement) || !summary.matches(':focus')) return;

  var row = summary.closest('details.concordance-row');
  if (!row) return;

  var root = row.getRootNode();
  var rows = Array.from(root.querySelectorAll('details.concordance-row'));
  var index = rows.indexOf(row);
  if (index < 0) return;

  if (event.key === 'ArrowDown' && index < rows.length - 1) {
    event.preventDefault();
    rows[index + 1].querySelector('summary').focus();
  } else if (event.key === 'ArrowUp' && index > 0) {
    event.preventDefault();
    rows[index - 1].querySelector('summary').focus();
  } else if (event.key === 'Escape' && row.open) {
    event.preventDefault();
    row.open = false;
    summary.focus();
  }
});
