package evidence

import "strings"

func Build(lines []EvidenceLine, finalGroup, staticNode string) Window {
	w := Window{
		Routes:      map[string]RouteObservation{},
		ErrorCounts: map[ErrorSource]int{},
	}
	for _, l := range lines {
		lower := strings.ToLower(l.Raw)
		if strings.TrimSpace(lower) == "" {
			continue
		}
		if strings.Contains(lower, "failed to get the second response from http://1.0.0.1") {
			w.HealthCheckOnly = true
			w.ErrorCounts[ErrHealthCheck]++
			continue
		}
		if transportOf(lower) == TransportUDP && !businessPort(lower) {
			continue
		}
		if ro, ok := parseRoute(l, finalGroup, staticNode); ok {
			key := string(ro.Target) + "|" + ro.Process + "|" + ro.Host
			if cur, exists := w.Routes[key]; !exists || routeIsNewer(ro, cur) {
				w.Routes[key] = ro
			}
		}
		if hasErrorToken(lower) && (strings.Contains(lower, "error") || strings.Contains(lower, "level=warning") || strings.Contains(lower, "level=error")) {
			src := classifyErrorSource(lower)
			w.ErrorCounts[src]++
			w.Errors = append(w.Errors, ErrorObservation{
				Source:    src,
				Target:    classifyTarget(lower),
				When:      parseLogTimestamp(l.Raw),
				Seq:       l.Seq,
				LogSource: l.Source,
				Raw:       l.Raw,
			})
		}
	}
	return w
}

func (w Window) LatestByTarget() map[Target]RouteObservation {
	out := map[Target]RouteObservation{}
	for _, ro := range w.Routes {
		if cur, ok := out[ro.Target]; !ok || routeIsNewer(ro, cur) {
			out[ro.Target] = ro
		}
	}
	return out
}
