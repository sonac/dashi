(function () {
  var PAD = { top: 10, right: 10, bottom: 10, left: 10 };
  var SVG_NS = 'http://www.w3.org/2000/svg';

  function svgEl(tag, attrs) {
    var el = document.createElementNS(SVG_NS, tag);
    for (var k in attrs) {
      el.setAttribute(k, attrs[k]);
    }
    return el;
  }

  function fmtValue(raw, metric, unit) {
    if (metric === 'MemUsedBytes') {
      return (raw / 1024 / 1024).toFixed(1) + ' ' + unit;
    }
    return raw.toFixed(1) + unit;
  }

  function fmtTime(date) {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  function initChart(svg) {
    var chartId = svg.getAttribute('data-chart-id');
    var containerId = svg.getAttribute('data-container-id');
    var metric = svg.getAttribute('data-metric');
    var unit = svg.getAttribute('data-unit') || '';
    var wrap = svg.closest('.chart-svg-wrap');
    var tooltip = wrap ? wrap.querySelector('[data-tooltip-for="' + chartId + '"]') : null;
    var toolbar = document.querySelector('.chart-toolbar[data-range-for="' + chartId + '"]');

    function draw(range) {
      fetch('/api/metrics/container/' + encodeURIComponent(containerId) + '?range=' + encodeURIComponent(range))
        .then(function (res) { return res.json(); })
        .then(function (rows) { render(rows || []); })
        .catch(function () { render([]); });
    }

    function render(rows) {
      while (svg.firstChild) {
        svg.removeChild(svg.firstChild);
      }
      var rect = svg.getBoundingClientRect();
      var w = rect.width || 600;
      var h = rect.height || 220;
      svg.setAttribute('viewBox', '0 0 ' + w + ' ' + h);

      if (!rows.length) {
        var empty = svgEl('text', { x: w / 2, y: h / 2, 'text-anchor': 'middle' });
        empty.setAttribute('style', 'fill:var(--muted); font-size:13px;');
        empty.textContent = 'No data yet';
        svg.appendChild(empty);
        return;
      }

      var plotW = w - PAD.left - PAD.right;
      var plotH = h - PAD.top - PAD.bottom;

      var values = rows.map(function (r) { return r[metric]; });
      var minV = Math.min.apply(null, values);
      var maxV = Math.max.apply(null, values);
      if (minV === maxV) {
        maxV = minV + 1;
      }

      var points = rows.map(function (r, i) {
        var x = PAD.left + (i / Math.max(rows.length - 1, 1)) * plotW;
        var y = PAD.top + plotH - ((r[metric] - minV) / (maxV - minV)) * plotH;
        return { x: x, y: y, ts: new Date(r.TS), value: r[metric] };
      });

      for (var g = 0; g <= 2; g++) {
        var gy = PAD.top + (plotH / 2) * g;
        svg.appendChild(svgEl('line', { x1: PAD.left, y1: gy, x2: w - PAD.right, y2: gy, class: 'chart-gridline' }));
      }

      var areaPts = points.map(function (p) { return p.x + ',' + p.y; }).join(' ');
      areaPts += ' ' + points[points.length - 1].x + ',' + (PAD.top + plotH) + ' ' + points[0].x + ',' + (PAD.top + plotH);
      var area = svgEl('polygon', { points: areaPts });
      area.setAttribute('style', 'fill:var(--accent-400); fill-opacity:0.1; stroke:none;');
      svg.appendChild(area);

      var linePts = points.map(function (p) { return p.x + ',' + p.y; }).join(' ');
      var line = svgEl('polyline', { points: linePts });
      line.setAttribute('style', 'fill:none; stroke:var(--accent-500); stroke-width:2px; stroke-linejoin:round; stroke-linecap:round;');
      svg.appendChild(line);

      var crosshair = svgEl('line', { x1: 0, y1: PAD.top, x2: 0, y2: PAD.top + plotH, class: 'chart-crosshair-line', visibility: 'hidden' });
      svg.appendChild(crosshair);

      var hit = svgEl('rect', { x: PAD.left, y: PAD.top, width: Math.max(plotW, 0), height: Math.max(plotH, 0), fill: 'transparent' });
      hit.addEventListener('mousemove', function (evt) {
        var box = svg.getBoundingClientRect();
        var mx = (evt.clientX - box.left) * (w / box.width);
        var nearest = points[0];
        var nearestDist = Infinity;
        for (var i = 0; i < points.length; i++) {
          var d = Math.abs(points[i].x - mx);
          if (d < nearestDist) {
            nearestDist = d;
            nearest = points[i];
          }
        }
        crosshair.setAttribute('x1', nearest.x);
        crosshair.setAttribute('x2', nearest.x);
        crosshair.setAttribute('visibility', 'visible');
        if (tooltip) {
          tooltip.textContent = fmtTime(nearest.ts) + ' · ' + fmtValue(nearest.value, metric, unit);
          tooltip.style.left = nearest.x + 'px';
          tooltip.style.top = nearest.y + 'px';
          tooltip.classList.add('visible');
        }
      });
      hit.addEventListener('mouseleave', function () {
        crosshair.setAttribute('visibility', 'hidden');
        if (tooltip) {
          tooltip.classList.remove('visible');
        }
      });
      svg.appendChild(hit);
    }

    if (toolbar) {
      toolbar.addEventListener('click', function (evt) {
        var btn = evt.target.closest ? evt.target.closest('button[data-range]') : null;
        if (!btn) {
          return;
        }
        var current = toolbar.querySelector('button.active');
        if (current) {
          current.classList.remove('active');
        }
        btn.classList.add('active');
        draw(btn.getAttribute('data-range'));
      });
    }

    draw(svg.getAttribute('data-range') || '1h');
  }

  document.addEventListener('DOMContentLoaded', function () {
    var charts = document.querySelectorAll('[data-chart]');
    for (var i = 0; i < charts.length; i++) {
      initChart(charts[i]);
    }
  });
})();
