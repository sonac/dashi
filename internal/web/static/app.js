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

    function fireInitialLoad() {
      if (window.htmx) {
        window.htmx.trigger(form, 'submit');
      }
    }
    // This script runs while the document is still parsing (it's the last
    // tag in <body>), before htmx's own DOMContentLoaded handler has bound
    // the form's hx-trigger="submit" listener — triggering here would be a
    // no-op. Defer to DOMContentLoaded (registered after htmx's own
    // listener, so it runs after htmx has bound the form) when still
    // parsing; call directly if this ever runs post-load instead.
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', fireInitialLoad);
    } else {
      fireInitialLoad();
    }
  }

  document.addEventListener('visibilitychange', syncVisibilityState);
  syncVisibilityState();
  setupLogsFilterPersistence();

  // Stop htmx polling requests while tab is hidden. htmx's interval timer
  // invokes the request handler without a synthetic triggering Event, so
  // this checks the element's own hx-trigger spec instead of event.type —
  // but an element with hx-trigger="load, every 15s" matches that check on
  // its very first (load) request too, so track first-load completion per
  // element and only start suppressing after that.
  var loadedOnce = new WeakSet();

  document.body.addEventListener('htmx:afterRequest', function (event) {
    var elt = event.detail && event.detail.elt;
    if (elt) {
      loadedOnce.add(elt);
    }
  });

  document.body.addEventListener('htmx:beforeRequest', function (event) {
    if (!document.hidden) {
      return;
    }
    var elt = event.detail && event.detail.elt;
    var spec = elt && elt.getAttribute && elt.getAttribute('hx-trigger');
    if (spec && spec.indexOf('every') !== -1 && loadedOnce.has(elt)) {
      event.preventDefault();
    }
  });
})();
