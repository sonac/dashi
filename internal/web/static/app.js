(function () {
  var logsKey = 'dashi.logsFilter.v1';

  function syncVisibilityState() {
    document.body.dataset.visibility = document.hidden ? 'hidden' : 'visible';
  }

  function saveLogsFilter(form) {
    var data = {};
    var fields = form.querySelectorAll('input[name], select[name]');
    for (var i = 0; i < fields.length; i++) {
      data[fields[i].name] = fields[i].value;
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
      // ignore storage/parsing failures
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

    if (window.htmx) {
      window.htmx.trigger(form, 'submit');
    }
  }

  document.addEventListener('visibilitychange', syncVisibilityState);
  syncVisibilityState();
  setupLogsFilterPersistence();

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
