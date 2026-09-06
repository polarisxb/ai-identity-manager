package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ai-identity-manager/internal/controller"
	"ai-identity-manager/internal/identity"
	"ai-identity-manager/internal/identity/evidence"
	"ai-identity-manager/internal/identity/session"
	"ai-identity-manager/internal/killswitch"
)

type commonOptions struct {
	config    string
	secret    string
	clashDir  string
	mixedPort string
	state     string
	json      bool
	verbose   bool
}

type statusSnapshot struct {
	cfg          identity.Config
	paths        identity.ProfilePaths
	report       identity.ChainReport
	window       evidence.Window
	sources      []string
	verify       identity.VerifyResult
	logsReadable bool
}

type stringList []string

const maxStatusEvidenceLines = 5

var openController = controller.Open

var newExitProbe = func(cfg identity.Config) identity.ExitProbe {
	return identity.MixedPortProbe{MixedPort: cfg.Clash.MixedPort}
}

var newVerifyProbe = func(cfg identity.Config, url string, timeout time.Duration) identity.ExitProbe {
	return identity.MixedPortProbe{MixedPort: cfg.Clash.MixedPort, URL: url, Timeout: timeout}
}

var newKillRunner = killswitch.NewSystemRunner

func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 1
	}
	switch args[0] {
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "apply":
		return runApply(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	case "watch":
		return runWatch(args[1:], stdout, stderr)
	case "monitor":
		return runMonitor(args[1:], stdout, stderr)
	case "reload":
		return runReload(args[1:], stdout, stderr)
	case "audit":
		return runAudit(args[1:], stdout, stderr)
	case "killswitch":
		return runKillSwitch(args[1:], stdout, stderr)
	case "switch-bootstrap":
		return runSwitchBootstrap(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 1
	}
}

func runKillSwitch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "killswitch requires status, apply, or remove")
		return 1
	}
	action := args[0]
	fs, opts := commandFlags("killswitch "+action, stderr)
	noPersist := false
	if action == "apply" {
		fs.BoolVar(&noPersist, "no-persist", false, "update firewall rules without installing the refresh task")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	cfg, err := loadCLIConfig(*opts)
	if err != nil {
		fmt.Fprintln(stderr, identity.Redact(err.Error(), cfg.SecretValues()))
		return 2
	}
	ksCfg := killSwitchConfig(cfg)
	if action == "apply" && !noPersist {
		persist, err := persistConfigForApply(*opts)
		if err != nil {
			printRedacted(stderr, cfg, "killswitch persist setup failed: %v\n", err)
			return 2
		}
		ksCfg.Persist = persist
	}
	ks := killswitch.New(ksCfg, newKillRunner())
	ctx := context.Background()
	switch action {
	case "status":
		status, err := ks.Status(ctx)
		if err != nil {
			printKillSwitchError(stderr, cfg, err)
			return 2
		}
		printKillSwitchStatus(stdout, cfg, status)
		if status.AllCovered && status.PersistInstalled {
			return 0
		}
		return 1
	case "apply":
		if err := ks.Apply(ctx); err != nil {
			printKillSwitchError(stderr, cfg, err)
			return 2
		}
		if noPersist {
			printRedacted(stdout, cfg, "killswitch applied\n")
		} else {
			printRedacted(stdout, cfg, "killswitch applied\nrefresh_task=%s every=%dm\n", killswitch.RefreshTaskName, killswitch.DefaultRefreshMinutes)
		}
		return 0
	case "remove":
		if err := ks.Remove(ctx); err != nil {
			printKillSwitchError(stderr, cfg, err)
			return 2
		}
		printRedacted(stdout, cfg, "killswitch removed\n")
		return 0
	default:
		fmt.Fprintf(stderr, "unknown killswitch command: %s\n", action)
		return 1
	}
}

func runReload(args []string, stdout, stderr io.Writer) int {
	fs, opts := commandFlags("reload", stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	cfg, err := loadCLIConfig(*opts)
	if err != nil {
		fmt.Fprintln(stderr, identity.Redact(err.Error(), cfg.SecretValues()))
		return 2
	}
	if strings.TrimSpace(cfg.Clash.ControllerSecret) == "" {
		appendAudit(*opts, cfg, "reload", "result=missing-secret")
		fmt.Fprintln(stderr, "controller secret is required; set CONTROLLER_SECRET in proxy-secret.local")
		return 2
	}
	ctx := context.Background()
	ctrl, err := openController(ctx, controller.Endpoint{
		TCPAddr:  cfg.Clash.Controller,
		PipePath: cfg.Clash.ControllerPipe,
		Secret:   cfg.Clash.ControllerSecret,
	})
	if err != nil {
		printControllerError(stderr, cfg, err)
		result := "controller-error"
		if errors.Is(err, controller.ErrNoController) {
			result = "unreachable"
		} else if errors.Is(err, controller.ErrUnauthorized) {
			result = "unauthorized"
		}
		appendAudit(*opts, cfg, "reload", "result=%s", result)
		return 2
	}
	defer ctrl.Close()
	runtimePath := filepath.Join(cfg.Clash.AppDir, cfg.Clash.RuntimeConfig)
	if err := ctrl.ReloadRuntime(ctx, runtimePath); err != nil {
		printRedacted(stderr, cfg, "reload failed: %v\n", err)
		appendAudit(*opts, cfg, "reload", "result=failed err=%v", err)
		return 2
	}
	conns, err := ctrl.Connections(ctx)
	if err != nil {
		printControllerError(stderr, cfg, err)
		appendAudit(*opts, cfg, "reload", "result=connections-failed err=%v", err)
		return 2
	}
	snap := session.Evaluate(conns, cfg.Nodes.FinalGroup, cfg.Nodes.StaticNode)
	if len(snap.AI) == 0 {
		printRedacted(stdout, cfg, "RELOAD done: no live AI connections to verify; run monitor to confirm\n")
		recordReload(*opts, cfg, "result=no-ai")
		return 1
	}
	if snap.AllStatic {
		printRedacted(stdout, cfg, "RELOAD ok: %d AI connections on AI-Static\n", len(snap.AI))
		recordReload(*opts, cfg, "result=ok ai=%d", len(snap.AI))
		return 0
	}
	for _, leak := range snap.Leaks {
		proc := leak.Conn.Metadata.Process
		if proc == "" {
			proc = leak.Conn.Metadata.ProcessPath
		}
		printRedacted(stdout, cfg, "leak target=%s host=%s process=%s chains=%s\n", leak.Target, leak.Conn.Metadata.Host, proc, strings.Join(leak.Conn.Chains, ">"))
	}
	printRedacted(stdout, cfg, "RELOAD stale: AI traffic not on AI-Static; re-apply the profile in Clash Verge Rev, then run monitor\n")
	recordReload(*opts, cfg, "result=stale leaks=%d", len(snap.Leaks))
	return 2
}

func runMonitor(args []string, stdout, stderr io.Writer) int {
	fs, opts := commandFlags("monitor", stderr)
	once := fs.Bool("once", false, "run one controller snapshot and exit")
	watch := fs.Bool("watch", false, "run controller snapshots until duration or interruption")
	interval := fs.Duration("interval", 5*time.Second, "watch poll interval")
	ipInterval := fs.Duration("ip-interval", 5*time.Minute, "exit identity sample interval")
	duration := fs.Duration("duration", 0, "watch duration; zero runs until interrupted")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if !*once && !*watch {
		fmt.Fprintln(stderr, "monitor requires --once or --watch")
		return 1
	}
	cfg, err := loadCLIConfig(*opts)
	if err != nil {
		fmt.Fprintln(stderr, identity.Redact(err.Error(), cfg.SecretValues()))
		return 2
	}
	if strings.TrimSpace(cfg.Clash.ControllerSecret) == "" {
		fmt.Fprintln(stderr, "controller secret is required; set CONTROLLER_SECRET in proxy-secret.local")
		return 2
	}
	ctx := context.Background()
	ctrl, err := openController(ctx, controller.Endpoint{
		TCPAddr:  cfg.Clash.Controller,
		PipePath: cfg.Clash.ControllerPipe,
		Secret:   cfg.Clash.ControllerSecret,
	})
	if err != nil {
		printControllerError(stderr, cfg, err)
		return 2
	}
	defer ctrl.Close()
	probe := newExitProbe(cfg)
	tr := session.NewTracker()
	seenChainErrors := map[string]bool{}
	if *once {
		if _, err := monitorStep(ctx, ctrl, probe, cfg, tr, nil, true, stdout); err != nil {
			printControllerError(stderr, cfg, err)
			return 2
		}
		scanLogErrorsStep(cfg, tr, seenChainErrors, stdout)
		return exitForLevel(tr.Worst())
	}
	var prev []controller.Connection
	deadline := time.Time{}
	if *duration > 0 {
		deadline = time.Now().Add(*duration)
	}
	lastIP := time.Now().Add(-*ipInterval)
	for {
		sampleIP := time.Since(lastIP) >= *ipInterval
		next, err := monitorStep(ctx, ctrl, probe, cfg, tr, prev, sampleIP, stdout)
		if err != nil {
			printControllerError(stderr, cfg, err)
		} else {
			prev = next
			scanLogErrorsStep(cfg, tr, seenChainErrors, stdout)
			if sampleIP {
				lastIP = time.Now()
			}
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			break
		}
		if !deadline.IsZero() && time.Until(deadline) < *interval {
			break
		}
		time.Sleep(*interval)
	}
	r := tr.Report()
	printRedacted(stdout, cfg, "session_summary baseline_ip=%s ip_changed=%t samples=%d sample_fails=%d drops=%d leaks=%d chain_errors=%d worst=%s\n",
		r.BaselineIP, r.IPChanged, r.Samples, r.SampleFails, r.Drops, r.LeakEvents, r.ChainErrors, r.Worst)
	return exitForLevel(tr.Worst())
}

func monitorStep(ctx context.Context, ctrl controller.Controller, probe identity.ExitProbe, cfg identity.Config, tr *session.Tracker, prev []controller.Connection, sampleIP bool, out io.Writer) ([]controller.Connection, error) {
	conns, err := ctrl.Connections(ctx)
	if err != nil {
		return prev, err
	}
	snap := session.Evaluate(conns, cfg.Nodes.FinalGroup, cfg.Nodes.StaticNode)
	tr.NoteSnapshot(snap)
	drops := session.DropsBetween(prev, conns)
	tr.NoteDrops(drops)
	if sampleIP {
		id, ferr := probe.Fetch(ctx)
		if ferr != nil {
			tr.NoteSampleFail()
			printRedacted(out, cfg, "exit_ip_sample_failed: %v\n", ferr)
		} else {
			v, reason := identity.ClassifyExit(id, cfg.Expected)
			tr.NoteExitIP(id, v)
			printRedacted(out, cfg, "exit_ip=%s asn=%s country=%s verdict=%s %s\n", id.IP, id.ASN, id.Country, exitVerdictStr(v), reason)
		}
	}
	allStatic := "YES"
	if !snap.AllStatic {
		allStatic = "NO"
	}
	printRedacted(out, cfg, "ai_connections=%d all_static=%s drops=%d\n", len(snap.AI), allStatic, len(drops))
	for _, leak := range snap.Leaks {
		proc := leak.Conn.Metadata.Process
		if proc == "" {
			proc = leak.Conn.Metadata.ProcessPath
		}
		printRedacted(out, cfg, "leak target=%s host=%s process=%s chains=%s\n",
			leak.Target, leak.Conn.Metadata.Host, proc, strings.Join(leak.Conn.Chains, ">"))
	}
	printRedacted(out, cfg, "reason_code=%s\n", monitorReasonCode(snap, tr.Report()))
	return conns, nil
}

func monitorReasonCode(snap session.Snapshot, report session.Report) identity.ReasonCode {
	if report.IPChanged {
		return identity.RSessionIPChanged
	}
	if len(snap.Leaks) > 0 {
		return identity.RAINonStatic
	}
	if len(snap.AI) == 0 {
		return identity.YNoAIHit
	}
	switch report.Worst {
	case identity.Red:
		return identity.RVerifyMismatch
	case identity.Yellow:
		return identity.YVerifyMissing
	default:
		return identity.GVerified
	}
}

func scanLogErrorsStep(cfg identity.Config, tr *session.Tracker, seen map[string]bool, out io.Writer) {
	files, _ := identity.DiscoverLogFiles(cfg.Clash.AppDir)
	text, _, _ := identity.ReadLogFiles(files, 256*1024)
	for _, ce := range identity.ScanChainErrors(text, cfg) {
		if seen[ce.Line] {
			continue
		}
		seen[ce.Line] = true
		tr.NoteChainError(ce.Source)
		printRedacted(out, cfg, "chain_error source=%s line=%s\n", ce.Source, ce.Line)
	}
}

func runStatus(args []string, stdout, stderr io.Writer) int {
	fs, opts := commandFlags("status", stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	snap, err := collectStatus(*opts)
	if err != nil {
		fmt.Fprintln(stderr, identity.Redact(err.Error(), snap.cfg.SecretValues()))
		return 2
	}
	status := evaluateSnapshot(*opts, snap)
	printStatus(stdout, snap.cfg, snap.paths, snap.report, snap.window, snap.sources, status, opts.json)
	return exitForLevel(status.Level)
}

func runApply(args []string, stdout, stderr io.Writer) int {
	fs, opts := commandFlags("apply", stderr)
	var bootstrap stringList
	fs.Var(&bootstrap, "bootstrap", "JMS bootstrap candidate node name; repeatable")
	noRules := fs.Bool("no-rules", false, "skip AI rule enhancement write")
	dryRun := fs.Bool("dry-run", false, "preview enhancement writes without modifying files")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	cfg, err := loadCLIConfig(*opts)
	if err != nil {
		fmt.Fprintln(stderr, identity.Redact(err.Error(), cfg.SecretValues()))
		return 2
	}
	paths, err := identity.ResolveProfilePaths(cfg.Clash.AppDir)
	if err != nil {
		fmt.Fprintln(stderr, identity.Redact(err.Error(), cfg.SecretValues()))
		return 2
	}
	if len(bootstrap) == 0 {
		fmt.Fprintln(stderr, "apply requires at least one --bootstrap JMS node")
		return 2
	}
	if *dryRun {
		writes, err := identity.PlanEnhancementWrites(paths, cfg, identity.ApplyOptions{
			BootstrapCandidates: bootstrap,
			NoRules:             *noRules,
		})
		if err != nil {
			fmt.Fprintln(stderr, identity.Redact(err.Error(), cfg.SecretValues()))
			return 2
		}
		for _, write := range writes {
			backup := ""
			if write.Exists && write.Changed {
				backup = write.BackupPath
			}
			fmt.Fprintf(stdout, "planned: changed=%t path=%s backup=%s\n", write.Changed, write.Path, backup)
		}
		return 0
	}
	result, err := identity.ApplyEnhancements(paths, cfg, identity.ApplyOptions{
		BootstrapCandidates: bootstrap,
		NoRules:             *noRules,
	})
	if err != nil {
		fmt.Fprintln(stderr, identity.Redact(err.Error(), cfg.SecretValues()))
		return 2
	}
	fmt.Fprintf(stdout, "APPLIED profile=%s changed=%d backups=%d\n", paths.ProfileUID, len(result.ChangedFiles), len(result.Backups))
	for _, path := range result.ChangedFiles {
		fmt.Fprintf(stdout, "changed: %s\n", path)
	}
	now := time.Now()
	_ = identity.RecordApply(activityPath(*opts), now)
	_ = identity.AppendAudit(auditPath(*opts), identity.AuditEvent{
		Time:    now,
		Type:    "apply",
		Summary: fmt.Sprintf("profile=%s changed=%d backups=%d", paths.ProfileUID, len(result.ChangedFiles), len(result.Backups)),
	}, cfg.SecretValues())
	return 0
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	fs, opts := commandFlags("doctor", stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	cfg, err := loadCLIConfig(*opts)
	if err != nil {
		fmt.Fprintf(stderr, "RED config: %s\n", identity.Redact(err.Error(), cfg.SecretValues()))
		return 2
	}
	fmt.Fprintf(stdout, "config: OK\n")
	fmt.Fprintf(stdout, "clash_dir: %s\n", cfg.Clash.AppDir)
	if _, err := os.Stat(cfg.Clash.AppDir); err != nil {
		fmt.Fprintf(stdout, "clash_dir_status: RED %v\n", err)
		return 2
	}
	paths, err := identity.ResolveProfilePaths(cfg.Clash.AppDir)
	if err != nil {
		fmt.Fprintf(stdout, "profile_status: RED %s\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "profile: %s\n", paths.ProfileUID)
	ctx := context.Background()
	checks := identity.RunDoctorChecks(cfg, paths, opts.state)
	checks = append(checks, checkController(ctx, cfg))
	checks = append(checks, checkKillSwitch(ctx, cfg))
	exitCode := 0
	for _, check := range checks {
		fmt.Fprintf(stdout, "check %s: %s %s\n", check.Name, check.Level, check.Message)
		if check.Level == identity.Red {
			exitCode = 2
		} else if check.Level == identity.Yellow && exitCode == 0 {
			exitCode = 1
		}
	}
	return exitCode
}

func checkController(ctx context.Context, cfg identity.Config) identity.DoctorCheck {
	ctrl, err := openController(ctx, controller.Endpoint{
		TCPAddr:  cfg.Clash.Controller,
		PipePath: cfg.Clash.ControllerPipe,
		Secret:   cfg.Clash.ControllerSecret,
	})
	if err != nil {
		return identity.DoctorCheck{
			Name:    "controller",
			Level:   identity.Yellow,
			Message: identity.Redact(err.Error(), cfg.SecretValues()),
		}
	}
	defer ctrl.Close()
	return identity.DoctorCheck{
		Name:    "controller",
		Level:   identity.Green,
		Message: "reachable via " + ctrl.Transport(),
	}
}

func checkKillSwitch(ctx context.Context, cfg identity.Config) identity.DoctorCheck {
	status, err := killswitch.New(killSwitchConfig(cfg), newKillRunner()).Status(ctx)
	if err != nil {
		return identity.DoctorCheck{Name: "killswitch", Level: identity.Yellow, Message: identity.Redact(err.Error(), cfg.SecretValues())}
	}
	persist := "missing"
	if status.PersistInstalled {
		persist = "installed"
	}
	if status.AllCovered && status.Covered > 0 && status.PersistInstalled {
		return identity.DoctorCheck{Name: "killswitch", Level: identity.Green, Message: fmt.Sprintf("covered=%d persist=installed", status.Covered)}
	}
	return identity.DoctorCheck{Name: "killswitch", Level: identity.Yellow, Message: fmt.Sprintf("covered=%d missing=%d persist=%s", status.Covered, status.Missing, persist)}
}

func runVerify(args []string, stdout, stderr io.Writer) int {
	fs, opts := commandFlags("verify", stderr)
	urlFlag := fs.String("url", "https://ipinfo.io/json", "identity verification URL")
	timeout := fs.Duration("timeout", 20*time.Second, "verify timeout")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	cfg, err := loadCLIConfig(*opts)
	if err != nil {
		fmt.Fprintln(stderr, identity.Redact(err.Error(), cfg.SecretValues()))
		return 2
	}
	probe := newVerifyProbe(cfg, *urlFlag, *timeout)
	id, ferr := probe.Fetch(context.Background())
	verify := identity.VerifyResult{
		Checked:         true,
		ExpectedExitIP:  cfg.Expected.ExitIP,
		ExpectedASN:     cfg.Expected.ASN,
		ExpectedCountry: cfg.Expected.Country,
	}
	if ferr != nil {
		verify.Error = ferr.Error()
	} else {
		verify.ExitIP = id.IP
		verify.ASN = id.ASN
		verify.Country = id.Country
	}
	verify.Error = identity.Redact(verify.Error, cfg.SecretValues())
	if err := identity.SaveVerifyResult(opts.state, verify, time.Now()); err != nil {
		fmt.Fprintln(stderr, identity.Redact(err.Error(), cfg.SecretValues()))
		return 2
	}
	level := identity.VerifyLevel(verify)
	if verify.Error != "" {
		fmt.Fprintf(stdout, "%s verify_error=%s\n", level, identity.Redact(verify.Error, cfg.SecretValues()))
	} else {
		fmt.Fprintf(stdout, "%s exit_ip=%s asn=%s country=%s\n", level, verify.ExitIP, verify.ASN, verify.Country)
	}
	appendAudit(*opts, cfg, "verify", "level=%s exit_ip=%s asn=%s country=%s err=%s", level, verify.ExitIP, verify.ASN, verify.Country, verify.Error)
	return exitForLevel(level)
}

func runWatch(args []string, stdout, stderr io.Writer) int {
	fs, opts := commandFlags("watch", stderr)
	interval := fs.Duration("interval", 5*time.Second, "status poll interval")
	once := fs.Bool("once", false, "run one status cycle and exit")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	for {
		snap, err := collectStatus(*opts)
		if err != nil {
			fmt.Fprintln(stderr, identity.Redact(err.Error(), snap.cfg.SecretValues()))
			return 2
		}
		status := evaluateSnapshot(*opts, snap)
		printStatus(stdout, snap.cfg, snap.paths, snap.report, snap.window, snap.sources, status, false)
		if *once {
			return exitForLevel(status.Level)
		}
		time.Sleep(*interval)
	}
}

func runSwitchBootstrap(args []string, stdout, stderr io.Writer) int {
	fs, opts := commandFlags("switch-bootstrap", stderr)
	node := fs.String("node", "", "JMS bootstrap node to move first")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if strings.TrimSpace(*node) == "" {
		fmt.Fprintln(stderr, "--node is required")
		return 1
	}
	cfg, err := loadCLIConfig(*opts)
	if err != nil {
		fmt.Fprintln(stderr, identity.Redact(err.Error(), cfg.SecretValues()))
		return 2
	}
	paths, err := identity.ResolveProfilePaths(cfg.Clash.AppDir)
	if err != nil {
		fmt.Fprintln(stderr, identity.Redact(err.Error(), cfg.SecretValues()))
		return 2
	}
	data, err := os.ReadFile(paths.GroupsFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	updated, err := identity.SwitchBootstrapInGroups(string(data), cfg.Nodes.BootstrapGroup, *node)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	backup := paths.GroupsFile + ".bak-ai-identity-" + time.Now().Format("20060102-150405")
	if err := os.WriteFile(backup, data, 0o600); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if err := os.WriteFile(paths.GroupsFile, []byte(updated), 0o600); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fmt.Fprintf(stdout, "SWITCHED bootstrap=%s backup=%s\n", *node, backup)
	appendAudit(*opts, cfg, "switch", "bootstrap=%s", *node)
	return 0
}

func runAudit(args []string, stdout, stderr io.Writer) int {
	fs, opts := commandFlags("audit", stderr)
	tail := fs.Int("tail", 20, "show last N audit entries")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	evs, err := identity.LoadRecentAudit(auditPath(*opts), *tail)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if len(evs) == 0 {
		fmt.Fprintln(stdout, "(no audit entries)")
		return 0
	}
	for _, ev := range evs {
		fmt.Fprintf(stdout, "%s %s %s\n", ev.Time.Format(time.RFC3339), ev.Type, ev.Summary)
	}
	return 0
}

func collectStatus(opts commonOptions) (statusSnapshot, error) {
	cfg, err := loadCLIConfig(opts)
	if err != nil {
		return statusSnapshot{cfg: cfg}, err
	}
	paths, err := identity.ResolveProfilePaths(cfg.Clash.AppDir)
	if err != nil {
		return statusSnapshot{cfg: cfg, paths: paths}, err
	}
	report, err := identity.InspectEnhancements(paths, cfg)
	if err != nil {
		return statusSnapshot{cfg: cfg, paths: paths, report: report}, err
	}
	files, _ := identity.DiscoverLogFiles(cfg.Clash.AppDir)
	logText, sources, _ := identity.ReadLogFiles(files, 256*1024)
	lines := evidence.LinesFromText("", logText, 0)
	window := evidence.Build(lines, cfg.Nodes.FinalGroup, cfg.Nodes.StaticNode)
	verify, err := identity.LoadVerifyResult(opts.state)
	if err != nil {
		return statusSnapshot{cfg: cfg, paths: paths, report: report, window: window, sources: sources, logsReadable: len(sources) > 0}, err
	}
	return statusSnapshot{
		cfg:          cfg,
		paths:        paths,
		report:       report,
		window:       window,
		sources:      sources,
		verify:       verify,
		logsReadable: len(sources) > 0,
	}, nil
}

func evaluateSnapshot(opts commonOptions, snap statusSnapshot) identity.StatusResult {
	activity, _ := identity.LoadActivity(activityPath(opts))
	return identity.EvaluateStatus(identity.StatusInputs{
		ConfigOK:            true,
		ChainOK:             snap.report.OK,
		Window:              snap.window,
		Clock:               identity.Clock{Now: time.Now(), LastApplyAt: activity.LastApplyAt, MaxAge: 30 * time.Minute},
		Verify:              snap.verify,
		ControllerReachable: true,
		LogsReadable:        snap.logsReadable,
		SwitchAfterFailures: snap.cfg.Bootstrap.SwitchAfterFailures,
	})
}

func loadCLIConfig(opts commonOptions) (identity.Config, error) {
	cfg, err := identity.LoadConfig(identity.LoadOptions{
		IdentityFile: opts.config,
		SecretFile:   opts.secret,
		ClashDir:     opts.clashDir,
	})
	if err != nil {
		return cfg, err
	}
	if opts.mixedPort != "" {
		cfg.Clash.MixedPort = opts.mixedPort
	}
	return cfg, nil
}

func auditPath(opts commonOptions) string {
	return filepath.Join(filepath.Dir(opts.state), "audit.log")
}

func activityPath(opts commonOptions) string {
	return filepath.Join(filepath.Dir(opts.state), "activity.json")
}

func appendAudit(opts commonOptions, cfg identity.Config, eventType, format string, args ...any) {
	_ = identity.AppendAudit(auditPath(opts), identity.AuditEvent{
		Type:    eventType,
		Summary: fmt.Sprintf(format, args...),
	}, cfg.SecretValues())
}

func recordReload(opts commonOptions, cfg identity.Config, format string, args ...any) {
	now := time.Now()
	_ = identity.RecordReload(activityPath(opts), now)
	_ = identity.AppendAudit(auditPath(opts), identity.AuditEvent{
		Time:    now,
		Type:    "reload",
		Summary: fmt.Sprintf(format, args...),
	}, cfg.SecretValues())
}

func commandFlags(name string, stderr io.Writer) (*flag.FlagSet, *commonOptions) {
	opts := &commonOptions{}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.config, "config", "", "identity.local.yaml path")
	fs.StringVar(&opts.secret, "secret", "", "proxy-secret.local path")
	fs.StringVar(&opts.clashDir, "clash-dir", "", "Clash Verge Rev app data directory")
	fs.StringVar(&opts.mixedPort, "mixed-port", "", "Clash mixed-port address")
	fs.StringVar(&opts.state, "state", ".ai-identity/status.json", "local status state file")
	fs.BoolVar(&opts.json, "json", false, "print JSON")
	fs.BoolVar(&opts.verbose, "verbose", false, "print verbose diagnostics")
	return fs, opts
}

func printStatus(stdout io.Writer, cfg identity.Config, paths identity.ProfilePaths, report identity.ChainReport, window evidence.Window, sources []string, status identity.StatusResult, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(stdout).Encode(map[string]any{
			"level":                status.Level,
			"reason_code":          status.Code,
			"reasons":              status.Reasons,
			"profile":              paths.ProfileUID,
			"missing":              report.Missing,
			"bootstrap_candidates": report.BootstrapCandidates,
			"evidence_logs":        sources,
			"evidence_lines":       redactedEvidenceLines(statusEvidenceLines(window, maxStatusEvidenceLines), cfg, maxStatusEvidenceLines),
		})
		return
	}
	fmt.Fprintf(stdout, "%s %s\n", status.Level, strings.Join(status.Reasons, "; "))
	fmt.Fprintf(stdout, "reason_code=%s\n", status.Code)
	fmt.Fprintf(stdout, "profile=%s final_group=%s static_node=%s bootstrap_group=%s\n", paths.ProfileUID, cfg.Nodes.FinalGroup, cfg.Nodes.StaticNode, cfg.Nodes.BootstrapGroup)
	if len(report.Missing) > 0 {
		fmt.Fprintf(stdout, "missing=%s\n", strings.Join(report.Missing, ", "))
	}
	if len(report.BootstrapCandidates) > 0 {
		fmt.Fprintf(stdout, "bootstrap_candidates=%s\n", strings.Join(report.BootstrapCandidates, ", "))
	}
	for _, source := range sources {
		fmt.Fprintf(stdout, "evidence_log=%s\n", source)
	}
	for _, line := range redactedEvidenceLines(statusEvidenceLines(window, maxStatusEvidenceLines), cfg, maxStatusEvidenceLines) {
		fmt.Fprintf(stdout, "evidence_line=%s\n", line)
	}
}

func statusEvidenceLines(window evidence.Window, limit int) []string {
	if limit <= 0 {
		return nil
	}
	type item struct {
		raw  string
		when time.Time
		seq  int
	}
	var items []item
	for _, ro := range window.Routes {
		if strings.TrimSpace(ro.Raw) != "" {
			items = append(items, item{raw: ro.Raw, when: ro.When, seq: ro.Seq})
		}
	}
	for _, err := range window.Errors {
		if strings.TrimSpace(err.Raw) != "" {
			items = append(items, item{raw: err.Raw, when: err.When, seq: err.Seq})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		if !left.when.IsZero() && !right.when.IsZero() && !left.when.Equal(right.when) {
			return left.when.After(right.when)
		}
		if !left.when.IsZero() && right.when.IsZero() {
			return true
		}
		if left.when.IsZero() && !right.when.IsZero() {
			return false
		}
		return left.seq > right.seq
	})
	out := make([]string, 0, min(limit, len(items)))
	for i, item := range items {
		if i >= limit {
			break
		}
		out = append(out, item.raw)
	}
	return out
}

func redactedEvidenceLines(lines []string, cfg identity.Config, limit int) []string {
	if limit <= 0 || len(lines) == 0 {
		return nil
	}
	if len(lines) > limit {
		lines = lines[:limit]
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, identity.Redact(line, cfg.SecretValues()))
	}
	return out
}

func printRedacted(w io.Writer, cfg identity.Config, format string, args ...any) {
	fmt.Fprint(w, identity.Redact(fmt.Sprintf(format, args...), cfg.SecretValues()))
}

func printControllerError(stderr io.Writer, cfg identity.Config, err error) {
	switch {
	case errors.Is(err, controller.ErrNoController):
		printRedacted(stderr, cfg, "controller unreachable: %v\n", err)
	case errors.Is(err, controller.ErrUnauthorized):
		printRedacted(stderr, cfg, "controller unauthorized: check controller secret\n")
	default:
		printRedacted(stderr, cfg, "controller error: %v\n", err)
	}
}

func killSwitchConfig(cfg identity.Config) killswitch.Config {
	return killswitch.Config{
		Enabled:  cfg.KillSwitch.Enabled,
		Appx:     cfg.KillSwitch.Appx,
		Globs:    cfg.KillSwitch.Globs,
		BlockUDP: cfg.KillSwitch.BlockUDP,
	}
}

func persistConfigForApply(opts commonOptions) (killswitch.PersistConfig, error) {
	exe, err := os.Executable()
	if err != nil {
		return killswitch.PersistConfig{}, err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return killswitch.PersistConfig{}, err
	}
	args := []string{"killswitch", "apply", "--no-persist"}
	for _, pair := range []struct {
		flag  string
		value string
	}{
		{"--secret", opts.secret},
		{"--config", opts.config},
		{"--clash-dir", opts.clashDir},
	} {
		if strings.TrimSpace(pair.value) == "" {
			continue
		}
		abs, err := filepath.Abs(pair.value)
		if err != nil {
			return killswitch.PersistConfig{}, err
		}
		args = append(args, pair.flag, abs)
	}
	return killswitch.PersistConfig{
		Enabled:    true,
		Executable: exe,
		Arguments:  joinTaskArguments(args),
		WorkingDir: filepath.Dir(exe),
	}, nil
}

func joinTaskArguments(args []string) string {
	parts := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\"") {
			parts = append(parts, `"`+strings.ReplaceAll(arg, `"`, `\"`)+`"`)
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

func printKillSwitchStatus(stdout io.Writer, cfg identity.Config, status killswitch.Status) {
	persist := "missing"
	if status.PersistInstalled {
		persist = "installed"
	}
	printRedacted(stdout, cfg, "killswitch targets=%d covered=%d missing=%d persist=%s\n", len(status.Targets), status.Covered, status.Missing, persist)
	for _, target := range status.Targets {
		state := "missing"
		if containsName(status.Plan.KeepNames, killSwitchRuleName(target)) {
			state = "covered"
		}
		if target.Kind == killswitch.KindAppx {
			printRedacted(stdout, cfg, "target label=%s kind=appx sid=%s status=%s\n", target.Label, target.PackageSID, state)
		} else {
			printRedacted(stdout, cfg, "target label=%s kind=program path=%s status=%s\n", target.Label, target.Program, state)
		}
	}
}

func printKillSwitchError(stderr io.Writer, cfg identity.Config, err error) {
	msg := err.Error()
	switch {
	case errors.Is(err, killswitch.ErrUnsupported):
		printRedacted(stderr, cfg, "killswitch is only supported on Windows\n")
	case strings.Contains(strings.ToLower(msg), "access is denied"), strings.Contains(strings.ToLower(msg), "access denied"):
		printRedacted(stderr, cfg, "killswitch failed: %v; 以管理员运行\n", err)
	default:
		printRedacted(stderr, cfg, "killswitch failed: %v\n", err)
	}
}

func killSwitchRuleName(target killswitch.Target) string {
	return killswitch.RulePrefix + " UDP " + target.Label
}

func containsName(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func exitVerdictStr(v identity.ExitVerdict) string {
	switch v {
	case identity.ExitMatched:
		return "matched"
	case identity.ExitMismatched:
		return "mismatched"
	default:
		return "unverifiable"
	}
}

func exitForLevel(level identity.Level) int {
	switch level {
	case identity.Green:
		return 0
	case identity.Yellow:
		return 1
	default:
		return 2
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: ai-identity-manager <status|apply|verify|watch|monitor|reload|audit|killswitch|switch-bootstrap|doctor> [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "monitor:")
	fmt.Fprintln(w, "  ai-identity-manager monitor --once --secret proxy-secret.local")
	fmt.Fprintln(w, "  ai-identity-manager monitor --watch --secret proxy-secret.local --interval 5s --ip-interval 5m")
	fmt.Fprintln(w, "  exit codes: 0 green, 1 unverifiable/sample failure, 2 leak/mismatch/controller failure")
	fmt.Fprintln(w, "  ai-identity-manager reload --secret proxy-secret.local")
	fmt.Fprintln(w, "  ai-identity-manager audit --tail 20")
	fmt.Fprintln(w, "  ai-identity-manager killswitch status|apply|remove")
	fmt.Fprintln(w, "  killswitch apply installs a logon/5m refresh task so Claude updates keep UDP blocked")
}

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}
