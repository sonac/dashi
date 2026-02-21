(function () {
  function pausePollingIfHidden() {
    if (document.hidden) {
      document.body.classList.add('paused');
    } else {
      document.body.classList.remove('paused');
    }
  }
  document.addEventListener('visibilitychange', pausePollingIfHidden);
  pausePollingIfHidden();
})();
