package web

import (
	"fmt"
	"html/template"
	"strings"
)

// renderSparkline builds a small inline SVG trend line for a stat tile:
// a 2px line, a ~10%-opacity area fill under it, and a ringed end-dot
// marking the latest value.
func renderSparkline(values []float64, w, h int) template.HTML {
	if len(values) == 0 {
		return template.HTML(fmt.Sprintf(`<svg viewBox="0 0 %d %d"></svg>`, w, h))
	}

	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	valRange := max - min
	if valRange == 0 {
		valRange = 1
	}

	const pad = 3.0
	usableH := float64(h) - 2*pad

	n := len(values)
	step := n - 1
	if step < 1 {
		step = 1
	}

	points := make([]string, n)
	var lastX, lastY float64
	for i, v := range values {
		x := float64(i) / float64(step) * float64(w)
		y := pad + usableH - ((v-min)/valRange)*usableH
		points[i] = fmt.Sprintf("%.1f,%.1f", x, y)
		lastX, lastY = x, y
	}
	line := strings.Join(points, " ")

	return template.HTML(fmt.Sprintf(
		`<svg viewBox="0 0 %d %d" preserveAspectRatio="none">`+
			`<polygon points="%s %.1f,%d 0,%d" fill="var(--accent-400)" fill-opacity="0.1" stroke="none"/>`+
			`<polyline points="%s" fill="none" stroke="var(--accent-500)" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>`+
			`<circle cx="%.1f" cy="%.1f" r="4" fill="var(--accent-500)" stroke="var(--surface)" stroke-width="2"/>`+
			`</svg>`,
		w, h,
		line, lastX, h, h,
		line,
		lastX, lastY,
	))
}
