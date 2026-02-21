(function () {
  var logsKey = 'dashi.logsFilter.v1';

  function pausePollingIfHidden() {
    if (document.hidden) {
      document.body.classList.add('paused');
    } else {
      document.body.classList.remove('paused');
    }
  }

  function saveLogsFilter(form) {
    var data = {};
    var fields = form.querySelectorAll('input[name], select[name]');
    for (var i = 0; i < fields.length; i++) {
      var el = fields[i];
      data[el.name] = el.value;
    }
    try {
      localStorage.setItem(logsKey, JSON.stringify(data));
    } catch (e) {
      // ignore storage failures
    }
  }

  function restoreLogsFilter(form) {
    try {
      var raw = localStorage.getItem(logsKey);
      if (!raw) {
        return;
      }
      var data = JSON.parse(raw);
      var fields = form.querySelectorAll('input[name], select[name]');
      for (var i = 0; i < fields.length; i++) {
        var el = fields[i];
        if (Object.prototype.hasOwnProperty.call(data, el.name)) {
          el.value = data[el.name];
        }
      }
    } catch (e) {
      // ignore parse/storage failures
    }
  }

  function setupLogsFilterPersistence() {
    var form = document.getElementById('logs-filter');
    if (!form) {
      return;
    }
    restoreLogsFilter(form);
    form.addEventListener('change', function () {
      saveLogsFilter(form);
    });
    form.addEventListener('submit', function () {
      saveLogsFilter(form);
    });
  }

  document.addEventListener('visibilitychange', pausePollingIfHidden);
  pausePollingIfHidden();
  setupLogsFilterPersistence();
})();
