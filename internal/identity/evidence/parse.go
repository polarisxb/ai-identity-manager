package evidence

import (
	"strings"
	"time"
)

func LinesFromText(source, text string, startSeq int) []EvidenceLine {
	var out []EvidenceLine
	seq := startSeq
	for _, raw := range strings.Split(text, "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		out = append(out, EvidenceLine{Source: source, Seq: seq, Raw: strings.TrimSpace(raw)})
		seq++
	}
	return out
}

func parseRoute(l EvidenceLine, finalGroup, staticNode string) (RouteObservation, bool) {
	lower := strings.ToLower(l.Raw)
	if !strings.Contains(lower, " using ") {
		return RouteObservation{}, false
	}
	target := classifyTarget(lower)
	if target == TargetNone {
		return RouteObservation{}, false
	}
	return RouteObservation{
		Target:    target,
		Process:   extractRouteProcess(lower),
		Host:      extractRouteHost(lower),
		Port:      extractRoutePort(lower),
		Transport: transportOf(lower),
		When:      parseLogTimestamp(l.Raw),
		Seq:       l.Seq,
		Source:    l.Source,
		Raw:       l.Raw,
		Kind:      classifyAIRoute(lower, finalGroup, staticNode),
	}, true
}

func classifyAIRoute(lower, finalGroup, staticNode string) RouteKind {
	staticDecision := "using " + strings.ToLower(finalGroup) + "[" + strings.ToLower(staticNode) + "]"
	switch {
	case strings.Contains(lower, staticDecision):
		return RouteStatic
	case strings.Contains(lower, "using direct"):
		return RouteDirect
	default:
		return RouteNonStatic
	}
}

func routeIsNewer(next, cur RouteObservation) bool {
	if !next.When.IsZero() && !cur.When.IsZero() && !next.When.Equal(cur.When) {
		return next.When.After(cur.When)
	}
	if !next.When.IsZero() && cur.When.IsZero() {
		return true
	}
	if next.When.IsZero() && !cur.When.IsZero() {
		return false
	}
	return next.Seq > cur.Seq
}

func extractRouteProcess(lower string) string {
	prefix := lower
	if arrow := strings.Index(lower, "-->"); arrow >= 0 {
		prefix = lower[:arrow]
	}
	open := strings.LastIndex(prefix, "(")
	close := strings.LastIndex(prefix, ")")
	if open < 0 || close <= open {
		return ""
	}
	return strings.TrimSpace(prefix[open+1 : close])
}

func extractRouteHost(lower string) string {
	arrow := strings.Index(lower, "-->")
	if arrow < 0 {
		return ""
	}
	rest := strings.TrimSpace(lower[arrow+3:])
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	target := strings.Trim(fields[0], `"'[],`)
	if strings.HasPrefix(target, "[") {
		if end := strings.Index(target, "]"); end >= 0 {
			return strings.Trim(target[1:end], `"'[],`)
		}
	}
	if colon := strings.LastIndex(target, ":"); colon > 0 {
		target = target[:colon]
	}
	return strings.Trim(target, `"'[],`)
}

func extractRoutePort(lower string) string {
	arrow := strings.Index(lower, "-->")
	if arrow < 0 {
		return ""
	}
	rest := strings.TrimSpace(lower[arrow+3:])
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	target := strings.Trim(fields[0], `"'[],`)
	if colon := strings.LastIndex(target, ":"); colon >= 0 && colon+1 < len(target) {
		return strings.Trim(target[colon+1:], `"'[],`)
	}
	return ""
}

func parseLogTimestamp(line string) time.Time {
	if idx := strings.Index(line, `time="`); idx >= 0 {
		start := idx + len(`time="`)
		if end := strings.Index(line[start:], `"`); end >= 0 {
			if parsed, err := time.Parse(time.RFC3339Nano, line[start:start+end]); err == nil {
				return parsed
			}
		}
	}
	if strings.HasPrefix(line, "[") {
		if end := strings.Index(line, "]"); end > 0 {
			value := line[1:end]
			for _, layout := range []string{"2006-01-02 15:04:05.000", "2006-01-02 15:04:05"} {
				if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
					return parsed
				}
			}
		}
	}
	return time.Time{}
}
