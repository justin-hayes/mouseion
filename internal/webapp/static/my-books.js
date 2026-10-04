window.showMyBooksNetworkRecovery = function (ctx) {
  if (!ctx || !ctx.sourceElement || !ctx.sourceElement.closest('[data-my-books-enhanced]') || ctx.response) return;
  var recovery = document.getElementById('library-recovery');
  if (!recovery) return;
  recovery.replaceChildren();
  var message = document.createElement('p');
  message.setAttribute('role', 'alert');
  message.textContent = 'My Books did not receive a response because the connection failed or the request timed out. Your current results and URL are unchanged. Retry the attempted search.';
  var retry = document.createElement('a');
  retry.href = ctx.request.action;
  retry.textContent = 'Retry My Books request';
  recovery.append(message, retry);
};

document.addEventListener('htmx:error', function (evt) {
  window.showMyBooksNetworkRecovery(evt.detail && evt.detail.ctx);
});
