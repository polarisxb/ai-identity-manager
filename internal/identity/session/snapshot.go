package session

import "ai-identity-manager/internal/controller"

type ConnVerdict struct {
	Conn       controller.Connection
	Target     Target
	OnStatic   bool
	ExitNode   string
	EntryGroup string
}

type Snapshot struct {
	Total     int
	AI        []ConnVerdict
	Leaks     []ConnVerdict
	AllStatic bool
	ByTarget  map[Target]int
}

func Evaluate(conns []controller.Connection, finalGroup, staticNode string) Snapshot {
	if finalGroup == "" {
		finalGroup = "AI-Static"
	}
	if staticNode == "" {
		staticNode = "IPRoyal-Static"
	}
	s := Snapshot{Total: len(conns), ByTarget: map[Target]int{}}
	for _, c := range conns {
		proc := c.Metadata.Process
		if proc == "" {
			proc = c.Metadata.ProcessPath
		}
		t := Classify(c.Metadata.Host, proc)
		if t == TargetNone {
			continue
		}
		v := ConnVerdict{
			Conn:     c,
			Target:   t,
			OnStatic: contains(c.Chains, staticNode) && contains(c.Chains, finalGroup),
		}
		if len(c.Chains) > 0 {
			v.ExitNode = c.Chains[0]
			v.EntryGroup = c.Chains[len(c.Chains)-1]
		}
		s.AI = append(s.AI, v)
		s.ByTarget[t]++
		if !v.OnStatic {
			s.Leaks = append(s.Leaks, v)
		}
	}
	s.AllStatic = len(s.Leaks) == 0
	return s
}

func contains(xs []string, x string) bool {
	for _, e := range xs {
		if e == x {
			return true
		}
	}
	return false
}
