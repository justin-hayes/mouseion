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

document.addEventListener('htmx:response:error', function (evt) {
  var ctx = evt.detail && evt.detail.ctx;
  if (!ctx || !ctx.sourceElement || !ctx.response || ctx.response.status !== 401) return;
  var recoveryScope = ctx.sourceElement.closest('#primary-goal-section, [data-my-books-enhanced], .library-search, form[action^="/library/"]');
  if (!recoveryScope) return;
  var request = ctx.request;
  var isSafeGet = request && request.method && request.method.toUpperCase() === 'GET';
  var next = isSafeGet && request.action
    ? request.action
    : window.location.pathname + window.location.search + window.location.hash;
  var loginURL = '/login?next=' + encodeURIComponent(next);
  if (!isSafeGet) {
    loginURL += '&error=' + encodeURIComponent('Your session expired. The action was not completed. Sign in and retry it.');
  }
  window.location.assign(loginURL);
});

document.addEventListener('htmx:error', function (evt) {
  window.showMyBooksNetworkRecovery(evt.detail && evt.detail.ctx);
});
