package session

import "ai-identity-manager/internal/identity"

type Tracker struct {
	baselineSet bool
	baselineIP  string
	ipChanged   bool
	samples     int
	sampleFails int
	drops       int
	leakEvents  int
	chainErrors int
	chainBySrc  map[identity.ErrorSource]int
	worst       identity.Level
}

type Report struct {
	BaselineIP    string
	IPChanged     bool
	Samples       int
	SampleFails   int
	Drops         int
	LeakEvents    int
	ChainErrors   int
	ChainBySource map[identity.ErrorSource]int
	Worst         identity.Level
}

func NewTracker() *Tracker {
	return &Tracker{worst: identity.Green}
}

func rank(l identity.Level) int {
	switch l {
	case identity.Red:
		return 2
	case identity.Yellow:
		return 1
	default:
		return 0
	}
}

func (t *Tracker) raise(l identity.Level) {
	if rank(l) > rank(t.worst) {
		t.worst = l
	}
}

func (t *Tracker) NoteSnapshot(s Snapshot) {
	if len(s.Leaks) > 0 {
		t.leakEvents += len(s.Leaks)
		t.raise(identity.Red)
	}
}

func (t *Tracker) NoteDrops(d []ConnVerdict) {
	t.drops += len(d)
}

func (t *Tracker) NoteChainError(src identity.ErrorSource) {
	t.chainErrors++
	if t.chainBySrc == nil {
		t.chainBySrc = map[identity.ErrorSource]int{}
	}
	t.chainBySrc[src]++
}

func (t *Tracker) NoteSampleFail() {
	t.sampleFails++
	t.raise(identity.Yellow)
}

func (t *Tracker) NoteExitIP(id identity.ExitIdentity, v identity.ExitVerdict) {
	t.samples++
	if !t.baselineSet {
		t.baselineSet = true
		t.baselineIP = id.IP
	} else if id.IP != t.baselineIP {
		t.ipChanged = true
		t.raise(identity.Red)
	}
	switch v {
	case identity.ExitUnverifiable:
		t.raise(identity.Yellow)
	case identity.ExitMismatched:
		t.raise(identity.Red)
	}
}

func (t *Tracker) Worst() identity.Level {
	return t.worst
}

func (t *Tracker) Report() Report {
	bySource := make(map[identity.ErrorSource]int, len(t.chainBySrc))
	for src, count := range t.chainBySrc {
		bySource[src] = count
	}
	return Report{
		BaselineIP:    t.baselineIP,
		IPChanged:     t.ipChanged,
		Samples:       t.samples,
		SampleFails:   t.sampleFails,
		Drops:         t.drops,
		LeakEvents:    t.leakEvents,
		ChainErrors:   t.chainErrors,
		ChainBySource: bySource,
		Worst:         t.worst,
	}
}
