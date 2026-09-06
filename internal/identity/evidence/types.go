package evidence

import "time"

type Transport int

const (
	TransportUnknown Transport = iota
	TransportTCP
	TransportUDP
)

type Target string

const (
	TargetNone   Target = ""
	TargetClaude Target = "claude"
	TargetOpenAI Target = "openai"
	TargetCodex  Target = "codex"
	TargetCursor Target = "cursor"
	TargetProbe  Target = "probe"
)

type RouteKind int

const (
	RouteUnknown RouteKind = iota
	RouteStatic
	RouteDirect
	RouteNonStatic
)

type ErrorSource int

const (
	ErrUnknown ErrorSource = iota
	ErrIPRoyal
	ErrJMS
	ErrHealthCheck
	ErrOrdinaryProxy
	ErrAIBusiness
)

type EvidenceLine struct {
	Source string
	Seq    int
	Raw    string
}

type RouteObservation struct {
	Target    Target
	Process   string
	Host      string
	Port      string
	Transport Transport
	Decision  string
	Group     string
	Node      string
	Kind      RouteKind
	When      time.Time
	Seq       int
	Source    string
	Raw       string
}

type ErrorObservation struct {
	Source    ErrorSource
	Target    Target
	When      time.Time
	Seq       int
	LogSource string
	Raw       string
}

type Window struct {
	Routes          map[string]RouteObservation
	Errors          []ErrorObservation
	ErrorCounts     map[ErrorSource]int
	HealthCheckOnly bool
}
