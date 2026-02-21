(function () {
  function syncVisibilityState() {
    document.body.dataset.visibility = document.hidden ? 'hidden' : 'visible';
  }

  document.addEventListener('visibilitychange', syncVisibilityState);
  syncVisibilityState();

  // Stop htmx polling requests while tab is hidden.
  document.body.addEventListener('htmx:beforeRequest', function (event) {
    if (document.hidden) {
      var trigger = event.detail && event.detail.requestConfig && event.detail.requestConfig.triggeringEvent;
      if (trigger && trigger.type === 'every') {
        event.preventDefault();
      }
    }
  });
})();
