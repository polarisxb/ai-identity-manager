package killswitch

import (
	"context"
	"errors"
	"strings"
)

var ErrUnsupported = errors.New("killswitch is only supported on windows")

type CommandRunner interface {
	Run(ctx context.Context, script string) (string, error)
}

type Config struct {
	Enabled  bool
	Appx     []string
	Globs    []string
	BlockUDP bool
	Persist  PersistConfig
}

type PersistConfig struct {
	Enabled         bool
	Executable      string
	Arguments       string
	WorkingDir      string
	IntervalMinutes int
}

type Manager struct {
	cfg    Config
	runner CommandRunner
}

type Kind int

const (
	KindAppx Kind = iota
	KindProgram
)

const RulePrefix = "AI-Identity-KillSwitch"
const RefreshTaskName = "AI-Identity-KillSwitch-Refresh"
const DefaultRefreshMinutes = 5

type Target struct {
	Kind       Kind
	Label      string
	PackageSID string
	Program    string
}

type Rule struct {
	Name       string
	PackageSID string
	Program    string
}

type Plan struct {
	Add         []Target
	RemoveNames []string
	KeepNames   []string
}

type Status struct {
	Targets            []Target
	Rules              []Rule
	Plan               Plan
	Covered            int
	Missing            int
	AllCovered         bool
	PersistInstalled   bool
	PersistTaskName    string
	PersistIntervalMin int
}

func New(cfg Config, runner CommandRunner) *Manager {
	return &Manager{cfg: cfg, runner: runner}
}

func (m *Manager) Resolve(ctx context.Context) ([]Target, error) {
	out, err := m.runner.Run(ctx, resolveScript(m.cfg.Appx, m.cfg.Globs))
	if err != nil {
		return nil, err
	}
	return parseTargets(out), nil
}

func (m *Manager) List(ctx context.Context) ([]Rule, error) {
	out, err := m.runner.Run(ctx, listScript())
	if err != nil {
		return nil, err
	}
	return parseRules(out), nil
}

func (m *Manager) Status(ctx context.Context) (Status, error) {
	targets, err := m.Resolve(ctx)
	if err != nil {
		return Status{}, err
	}
	rules, err := m.List(ctx)
	if err != nil {
		return Status{}, err
	}
	plan := ComputePlan(targets, rules)
	persistOut, err := m.runner.Run(ctx, persistStatusScript())
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Targets:            targets,
		Rules:              rules,
		Plan:               plan,
		Covered:            len(plan.KeepNames),
		Missing:            len(plan.Add),
		AllCovered:         len(plan.Add) == 0,
		PersistInstalled:   parsePersistInstalled(persistOut),
		PersistTaskName:    RefreshTaskName,
		PersistIntervalMin: persistIntervalMinutes(m.cfg.Persist),
	}
	return status, nil
}

func (m *Manager) Apply(ctx context.Context) error {
	targets, err := m.Resolve(ctx)
	if err != nil {
		return err
	}
	rules, err := m.List(ctx)
	if err != nil {
		return err
	}
	plan := ComputePlan(targets, rules)
	for _, name := range plan.RemoveNames {
		if _, err := m.runner.Run(ctx, removeRuleScript(name)); err != nil {
			return err
		}
	}
	for _, target := range plan.Add {
		if _, err := m.runner.Run(ctx, addRuleScript(target)); err != nil {
			return err
		}
	}
	if m.cfg.Persist.Enabled {
		if strings.TrimSpace(m.cfg.Persist.Executable) == "" {
			return errors.New("killswitch persist executable is required")
		}
		if _, err := m.runner.Run(ctx, installPersistScript(m.cfg.Persist)); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Remove(ctx context.Context) error {
	rules, err := m.List(ctx)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if _, err := m.runner.Run(ctx, removeRuleScript(rule.Name)); err != nil {
			return err
		}
	}
	if _, err := m.runner.Run(ctx, removePersistScript()); err != nil {
		return err
	}
	return nil
}

func ruleName(t Target) string {
	return RulePrefix + " UDP " + t.Label
}

func ruleMatchesTarget(r Rule, t Target) bool {
	if t.Kind == KindAppx {
		return r.PackageSID != "" && r.PackageSID == t.PackageSID
	}
	return r.Program != "" && r.Program == t.Program
}

func ComputePlan(targets []Target, existing []Rule) Plan {
	byName := map[string]Rule{}
	for _, r := range existing {
		byName[r.Name] = r
	}
	used := map[string]bool{}
	var p Plan
	for _, t := range targets {
		name := ruleName(t)
		used[name] = true
		r, ok := byName[name]
		if ok && ruleMatchesTarget(r, t) {
			p.KeepNames = append(p.KeepNames, name)
			continue
		}
		if ok {
			p.RemoveNames = append(p.RemoveNames, name)
		}
		p.Add = append(p.Add, t)
	}
	for _, r := range existing {
		if !used[r.Name] {
			p.RemoveNames = append(p.RemoveNames, r.Name)
		}
	}
	return p
}

func parseTargets(text string) []Target {
	var targets []Target
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		kind := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		label := strings.TrimSpace(parts[2])
		switch kind {
		case "appx":
			if value != "" {
				targets = append(targets, Target{Kind: KindAppx, Label: label, PackageSID: value})
			}
		case "program":
			if value != "" {
				targets = append(targets, Target{Kind: KindProgram, Label: label, Program: value})
			}
		}
	}
	return targets
}

func parseRules(text string) []Rule {
	var rules []Rule
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		rules = append(rules, Rule{
			Name:       name,
			PackageSID: strings.TrimSpace(parts[1]),
			Program:    strings.TrimSpace(parts[2]),
		})
	}
	return rules
}
