package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"omastack/internal/bounded"
	"omastack/internal/control"
	"omastack/internal/daemon"
	"omastack/internal/deps"
	"omastack/internal/docker"
	"omastack/internal/install"
	"omastack/internal/model"
	"omastack/internal/paths"
	"omastack/internal/store"
	"omastack/internal/supervise"
	"omastack/internal/validate"
)

const version = "0.1.2"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "omastack:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	resolved, err := paths.Resolve()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	case "daemon":
		instance, err := daemon.New(resolved)
		if err != nil {
			return err
		}
		return instance.Run(ctx)
	case "supervise":
		if len(args) != 2 {
			return errors.New("usage: omastack supervise <service-id>")
		}
		return supervise.Run(ctx, resolved, args[1])
	case "docker-shell":
		if len(args) != 3 || !validate.ID(args[1]) || !filepath.IsAbs(args[2]) {
			return errors.New("usage: omastack docker-shell <service-id> <absolute-config-path>")
		}
		// A terminal broker may restore different XDG settings. Bind the helper
		// to the daemon's exact private config, never secret-bearing argv.
		config, err := store.OpenConfig(args[2])
		if err != nil {
			return err
		}
		_, service, ok := config.Get().FindService(args[1])
		if !ok {
			return errors.New("docker service not found")
		}
		return docker.RunShell(ctx, *service)
	case "list":
		return showStatus(ctx, resolved, false, args[1:])
	case "status":
		return showStatus(ctx, resolved, true, args[1:])
	case "start", "stop", "restart":
		if len(args) != 2 {
			return fmt.Errorf("usage: omastack %s <project-or-service>", args[0])
		}
		return callAction(ctx, resolved, args[0], args[1])
	case "kill":
		if len(args) != 2 {
			return errors.New("usage: omastack kill <service>")
		}
		return callAction(ctx, resolved, "kill", args[1])
	case "logs":
		return logsCommand(ctx, resolved, args[1:])
	case "doctor":
		return printCall(ctx, resolved, "doctor", map[string]any{})
	case "cleanup":
		return printCall(ctx, resolved, "cleanup", map[string]any{})
	case "export":
		return printCall(ctx, resolved, "config.export", map[string]any{})
	case "import":
		if len(args) != 3 || args[2] != "--replace" {
			return errors.New("usage: omastack import <backup-path> --replace")
		}
		absolute, err := filepath.Abs(args[1])
		if err != nil {
			return err
		}
		return printCall(ctx, resolved, "config.import", map[string]any{"path": filepath.Clean(absolute), "replace": true})
	case "docker":
		return dockerCommand(ctx, resolved, args[1:])
	case "open":
		return openPanel()
	case "request":
		return requestCommand(ctx, resolved, args[1:])
	case "setup":
		result, err := install.Setup(ctx, resolved)
		if err != nil {
			return err
		}
		return printJSON(result)
	case "uninstall":
		return uninstallCommand(ctx, resolved, args[1:])
	case "help", "--help", "-h":
		return usage()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() error {
	fmt.Print(`OmaStack — Your local development stack, one click away.

Usage:
  omastack list [--json]
  omastack status [--json]
  omastack start <project-or-service>
  omastack stop <project-or-service>
  omastack restart <project-or-service>
  omastack logs <service-or-project> [--lines N] [--query TEXT] [--follow]
  omastack doctor
  omastack cleanup
  omastack export
  omastack import <backup-path> --replace
  omastack docker <rebuild|recreate|terminal> <service>
  omastack open
  omastack setup
  omastack uninstall [--purge] [--yes]
`)
	return nil
}

func dockerCommand(ctx context.Context, resolved paths.Paths, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: omastack docker <rebuild|recreate|terminal> <service>")
	}
	action, service := args[0], args[1]
	if action == "terminal" {
		return printCall(ctx, resolved, "docker.terminal", map[string]string{"serviceId": service})
	}
	if action != "rebuild" && action != "recreate" {
		return errors.New("docker action must be rebuild, recreate, or terminal")
	}
	return printCall(ctx, resolved, "docker.action", map[string]string{"serviceId": service, "action": action})
}

func client(resolved paths.Paths) control.Client {
	return control.Client{Socket: resolved.SocketFile, Timeout: 30 * time.Second}
}

func clientForMethod(resolved paths.Paths, method string) control.Client {
	result := client(resolved)
	if method == "stop" || method == "restart" {
		result.Timeout = 12*time.Hour + 10*time.Second
		return result
	}
	if method == "start" || method == "restart" || method == "docker.action" {
		result.Timeout = 6 * time.Minute
	} else if method == "stop" || method == "kill" || method == "config.import" {
		result.Timeout = 2 * time.Minute
	}
	return result
}

func callAction(ctx context.Context, resolved paths.Paths, action, target string) error {
	var result map[string]string
	if err := clientForMethod(resolved, action).Call(ctx, action, control.TargetParams{Target: target}, &result); err != nil {
		return err
	}
	fmt.Println(result["status"])
	return nil
}

func showStatus(ctx context.Context, resolved paths.Paths, detailed bool, args []string) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	var snapshot model.Snapshot
	if err := client(resolved).Call(ctx, "status", map[string]any{}, &snapshot); err != nil {
		return err
	}
	if *asJSON {
		return printJSON(snapshot)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tSERVICE\tSTATE\tPID\tCPU\tMEMORY\tPORTS\tUPTIME")
	for _, project := range snapshot.Projects {
		for _, service := range project.Services {
			runtime := snapshot.Runtime[service.ID]
			ports := make([]string, len(runtime.Ports))
			for i, port := range runtime.Ports {
				ports[i] = fmt.Sprint(port)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%.1f%%\t%.1f MiB\t%s\t%s\n", project.Name, service.Name, runtime.Status, runtime.PID, runtime.CPU, runtime.MemoryMB, strings.Join(ports, ","), duration(runtime.UptimeSeconds))
			if detailed && runtime.LastError != "" {
				fmt.Fprintf(w, "\t↳ %s\n", runtime.LastError)
			}
		}
	}
	return w.Flush()
}

func logsCommand(ctx context.Context, resolved paths.Paths, args []string) error {
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	lines := flags.Int("lines", 200, "maximum lines")
	query := flags.String("query", "", "case-insensitive search")
	asJSON := flags.Bool("json", false, "print JSON")
	follow := flags.Bool("follow", false, "continue following logs")
	metadata := flags.Bool("metadata", false, "include log-limit metadata with --json")
	target := ""
	flagArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		target, flagArgs = args[0], args[1:]
	}
	if err := flags.Parse(flagArgs); err != nil {
		return err
	}
	if target == "" && flags.NArg() == 1 {
		target = flags.Arg(0)
	}
	if target == "" || flags.NArg() > 1 || (len(args) > 0 && !strings.HasPrefix(args[0], "-") && flags.NArg() > 0) {
		return errors.New("usage: omastack logs <service-or-project> [--lines N] [--query TEXT] [--follow]")
	}
	if *asJSON && *follow {
		return errors.New("--json and --follow cannot be combined")
	}
	if *metadata && !*asJSON {
		return errors.New("--metadata requires --json")
	}
	params := control.LogsParams{Target: target, Lines: *lines, Query: *query}
	seen := map[string]bool{}
	printEntries := func() error {
		var result struct {
			Entries   []map[string]any `json:"entries"`
			Truncated bool             `json:"truncated"`
			Limit     int              `json:"limit"`
			Notice    string           `json:"notice,omitempty"`
		}
		params.Metadata = true
		if err := client(resolved).Call(ctx, "logs", params, &result); err != nil {
			return err
		}
		entries := result.Entries
		if *asJSON {
			if *metadata {
				return printJSON(result)
			}
			return printJSON(entries)
		}
		if result.Truncated {
			fmt.Fprintln(os.Stderr, "omastack:", result.Notice)
		}
		for _, entry := range entries {
			key := fmt.Sprint(entry["timestamp"], "\x00", entry["serviceId"], "\x00", entry["stream"], "\x00", entry["message"])
			if seen[key] {
				continue
			}
			seen[key] = true
			label := fmt.Sprint(entry["service"])
			if label != "<nil>" && label != "" {
				label += "  "
			} else {
				label = ""
			}
			fmt.Printf("%s  %-6s  %s%s\n", entry["timestamp"], entry["stream"], label, entry["message"])
		}
		return nil
	}
	if err := printEntries(); err != nil {
		return err
	}
	if !*follow {
		return nil
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := printEntries(); err != nil {
				return err
			}
			if len(seen) > 20000 {
				seen = map[string]bool{}
			}
		}
	}
}

func requestCommand(ctx context.Context, resolved paths.Paths, args []string) error {
	params, err := requestParams(args, os.Stdin)
	if err != nil {
		return err
	}
	var result any
	if err := clientForMethod(resolved, args[0]).Call(ctx, args[0], params, &result); err != nil {
		return err
	}
	return printJSON(result)
}

func requestParams(args []string, input io.Reader) (json.RawMessage, error) {
	if len(args) < 1 || len(args) > 2 {
		return nil, errors.New("usage: omastack request <method> [params-json|-] ('-' reads one JSON line from stdin)")
	}
	params := json.RawMessage("{}")
	if len(args) == 2 {
		params = json.RawMessage(args[1])
		if args[1] == "-" {
			line, err := bufio.NewReader(io.LimitReader(input, control.MaxMessageBytes/2+2)).ReadBytes('\n')
			if err != nil && err != io.EOF {
				return nil, errors.New("could not read request parameters from stdin")
			}
			params = json.RawMessage(strings.TrimSuffix(string(line), "\n"))
		}
	}
	if len(params) > control.MaxMessageBytes/2 {
		return nil, errors.New("params-json exceeds request size limit")
	}
	if !json.Valid(params) {
		return nil, errors.New("params-json is invalid")
	}
	return params, nil
}

func printCall(ctx context.Context, resolved paths.Paths, method string, params any) error {
	var result any
	if err := clientForMethod(resolved, method).Call(ctx, method, params, &result); err != nil {
		return err
	}
	return printJSON(result)
}

func openPanel() error {
	cmd := exec.Command("omarchy-shell", "shell", "summon", "david.omastack", "{}")
	output := bounded.NewBuffer(64 << 10)
	cmd.Stdout, cmd.Stderr = output, output
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("open panel: %s", strings.TrimSpace(output.String()))
	}
	return nil
}

func uninstallCommand(ctx context.Context, resolved paths.Paths, args []string) error {
	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	purge := flags.Bool("purge", false, "delete all OmaStack data")
	yes := flags.Bool("yes", false, "confirm non-interactively")
	if err := flags.Parse(args); err != nil {
		return err
	}
	storeInstance, err := store.OpenConfig(resolved.ConfigFile)
	if err != nil {
		return fmt.Errorf("read configuration before uninstall: %w", err)
	}
	services := storeInstance.Get().AllServices()
	targets := make([]string, 0, len(services))
	for id := range services {
		targets = append(targets, id)
	}
	sort.Strings(targets)
	serviceIDs, err := deps.ShutdownOrder(services, targets)
	if err != nil {
		return fmt.Errorf("plan uninstall shutdown: %w", err)
	}
	if *purge && !*yes {
		fmt.Fprint(os.Stderr, "Type PURGE to permanently delete OmaStack project definitions and state: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(line) != "PURGE" {
			return errors.New("purge canceled")
		}
	}
	if err := install.Uninstall(ctx, resolved, serviceIDs, true); err != nil {
		return err
	}
	if *purge {
		if err := install.Purge(resolved); err != nil {
			return err
		}
	}
	fmt.Println("OmaStack user services removed. Plugin source is removed separately with: omarchy plugin remove david.omastack")
	if !*purge {
		fmt.Println("Project definitions were preserved in", resolved.ConfigDir)
	}
	return nil
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func duration(seconds int64) string {
	if seconds <= 0 {
		return "-"
	}
	value := time.Duration(seconds) * time.Second
	if value >= 24*time.Hour {
		return fmt.Sprintf("%dd%dh", int(value/(24*time.Hour)), int(value%(24*time.Hour)/time.Hour))
	}
	if value >= time.Hour {
		return fmt.Sprintf("%dh%dm", int(value/time.Hour), int(value%time.Hour/time.Minute))
	}
	if value >= time.Minute {
		return fmt.Sprintf("%dm%ds", int(value/time.Minute), int(value%time.Minute/time.Second))
	}
	return fmt.Sprintf("%ds", seconds)
}
