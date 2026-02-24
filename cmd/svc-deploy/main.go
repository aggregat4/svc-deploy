package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/a4/svc-deploy/internal/config"
	"github.com/a4/svc-deploy/internal/deploy"
	"github.com/a4/svc-deploy/internal/prune"
	"github.com/a4/svc-deploy/internal/rollback"
	"github.com/a4/svc-deploy/internal/status"
)

const version = "0.1.0"

type realClock struct{}

func (realClock) Now() time.Time                  { return time.Now() }
func (realClock) Since(t time.Time) time.Duration { return time.Since(t) }

// cliFlags holds parsed global flags
type cliFlags struct {
	configPath string
	jsonOutput bool
	showVer    bool
	showHelp   bool
}

type jsonErrorResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

type jsonDeployResponse struct {
	Success         bool   `json:"success"`
	Version         string `json:"version"`
	PreviousVersion string `json:"previous_version"`
}

type jsonStatusResponse struct {
	Success         bool   `json:"success"`
	Service         string `json:"service"`
	CurrentVersion  string `json:"current_version"`
	PreviousVersion string `json:"previous_version"`
	Active          bool   `json:"active"`
}

type jsonPruneResponse struct {
	Success   bool `json:"success"`
	Removed   int  `json:"removed"`
	Remaining int  `json:"remaining"`
}

// writeJSON writes a JSON payload to stdout.
func writeJSON(payload any) {
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(payload)
}

// parseGlobalFlags extracts global flags and returns remaining args.
// Global flags are supported before or after the command.
// Unknown flags before the command are rejected; unknown flags after the
// command are preserved for subcommand parsing.
func parseGlobalFlags(args []string) (cliFlags, []string, error) {
	var flags cliFlags
	var remaining []string
	commandSeen := false

	isGlobalFlag := func(arg string) bool {
		switch arg {
		case "--config", "-c", "--json", "-j", "--version", "-v", "--help", "-h":
			return true
		default:
			return false
		}
	}

	i := 0
	for i < len(args) {
		arg := args[i]

		if arg == "--" {
			remaining = append(remaining, args[i+1:]...)
			break
		}

		if isGlobalFlag(arg) {
			switch arg {
			case "--config", "-c":
				if i+1 >= len(args) {
					return flags, nil, fmt.Errorf("flag %s requires an argument", arg)
				}
				flags.configPath = args[i+1]
				i += 2
			case "--json", "-j":
				flags.jsonOutput = true
				i++
			case "--version", "-v":
				flags.showVer = true
				i++
			case "--help", "-h":
				flags.showHelp = true
				i++
			}
			continue
		}

		if strings.HasPrefix(arg, "-") && !commandSeen {
			return flags, nil, fmt.Errorf("unknown flag: %s", arg)
		}

		commandSeen = true
		remaining = append(remaining, arg)
		i++
	}

	return flags, remaining, nil
}

func main() {
	// Parse global flags from args (supports before/after command)
	flags, remaining, err := parseGlobalFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Handle help/version before config loading
	if flags.showHelp {
		printUsage()
		os.Exit(0)
	}
	if flags.showVer {
		fmt.Println("svc-deploy", version)
		os.Exit(0)
	}

	// Must have a command
	if len(remaining) < 1 {
		fmt.Fprintln(os.Stderr, "error: no command specified")
		printUsage()
		os.Exit(1)
	}

	cmd := remaining[0]
	cmdArgs := remaining[1:]

	// Load configuration (not needed for help, already handled above)
	var cfg *config.DeployMap
	if configPath := flags.configPath; configPath != "" {
		cfg, err = config.Load(configPath)
	} else {
		cfg, err = config.LoadFromDefaultPaths()
	}
	if err != nil {
		if flags.jsonOutput {
			writeJSON(jsonErrorResponse{
				Success: false,
				Error:   fmt.Sprintf("loading config: %v", err),
			})
		} else {
			fmt.Fprintf(os.Stderr, "error: loading config: %v\n", err)
		}
		os.Exit(1)
	}

	// Create real implementations
	clock := realClock{}
	fs := NewRealFS()
	fetcher := NewHTTPArtifactFetcher()
	locker := NewFileLocker()
	svcMgr := NewSystemdManager()
	healthChecker := NewHTTPHealthChecker()
	symlinkMgr := NewAtomicSymlinkManager(fs)
	configRepo := NewGitConfigRepo("/opt/config-repo")

	ctx := context.Background()

	switch cmd {
	case "deploy":
		if len(cmdArgs) < 2 {
			fmt.Fprintln(os.Stderr, "usage: svc-deploy deploy <service> <version>")
			os.Exit(1)
		}
		service := cmdArgs[0]
		ver := cmdArgs[1]
		exitCode := runDeploy(ctx, cfg, service, ver, deploy.Deps{
			FS:            fs,
			Fetcher:       fetcher,
			Locker:        locker,
			ServiceMgr:    svcMgr,
			HealthChecker: healthChecker,
			SymlinkMgr:    symlinkMgr,
			ConfigRepo:    configRepo,
			Clock:         clock,
		}, flags.jsonOutput)
		os.Exit(exitCode)

	case "rollback":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(os.Stderr, "usage: svc-deploy rollback <service> [target-version]")
			os.Exit(1)
		}
		service := cmdArgs[0]
		targetVersion := ""
		if len(cmdArgs) >= 2 {
			targetVersion = cmdArgs[1]
		}
		exitCode := runRollback(ctx, cfg, service, targetVersion, rollback.Deps{
			FS:            fs,
			Locker:        locker,
			ServiceMgr:    svcMgr,
			HealthChecker: healthChecker,
			SymlinkMgr:    symlinkMgr,
			Clock:         clock,
		}, flags.jsonOutput)
		os.Exit(exitCode)

	case "status":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(os.Stderr, "usage: svc-deploy status <service>")
			os.Exit(1)
		}
		service := cmdArgs[0]
		exitCode := runStatus(ctx, cfg, service, status.Deps{
			FS:         fs,
			ServiceMgr: svcMgr,
		}, flags.jsonOutput)
		os.Exit(exitCode)

	case "prune":
		if len(cmdArgs) < 1 {
			fmt.Fprintln(os.Stderr, "usage: svc-deploy prune <service> [--keep N]")
			os.Exit(1)
		}
		service := cmdArgs[0]
		keep := 0 // 0 means use config default
		for i := 1; i < len(cmdArgs); i++ {
			if cmdArgs[i] == "--keep" && i+1 < len(cmdArgs) {
				keep = parseInt(cmdArgs[i+1])
				break
			}
		}
		exitCode := runPrune(ctx, cfg, service, keep, prune.Deps{
			FS: fs,
		}, flags.jsonOutput)
		os.Exit(exitCode)

	case "help":
		printUsage()
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "error: unknown command %q\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func runDeploy(ctx context.Context, cfg *config.DeployMap, service, version string, deps deploy.Deps, jsonOutput bool) int {
	svcCfg, ok := cfg.GetService(service)
	if !ok {
		if jsonOutput {
			writeJSON(jsonErrorResponse{
				Success: false,
				Error:   fmt.Sprintf("service %q not found in deploy map", service),
			})
		} else {
			fmt.Fprintf(os.Stderr, "error: service %q not found in deploy map\n", service)
		}
		return 1
	}

	op := deploy.New(svcCfg, service, version, deps)
	result, err := op.Run(ctx)

	if jsonOutput {
		if err != nil {
			writeJSON(jsonErrorResponse{Success: false, Error: err.Error()})
		} else {
			writeJSON(jsonDeployResponse{
				Success:         true,
				Version:         result.Version,
				PreviousVersion: result.PreviousVersion,
			})
		}
	} else {
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: deploy failed: %v\n", err)
			return 1
		}
		fmt.Printf("Deployed %s@%s\n", service, result.Version)
		if result.PreviousVersion != "" {
			fmt.Printf("Previous version: %s\n", result.PreviousVersion)
		}
	}

	if err != nil {
		return 1
	}
	return 0
}

func runRollback(ctx context.Context, cfg *config.DeployMap, service, targetVersion string, deps rollback.Deps, jsonOutput bool) int {
	svcCfg, ok := cfg.GetService(service)
	if !ok {
		if jsonOutput {
			writeJSON(jsonErrorResponse{
				Success: false,
				Error:   fmt.Sprintf("service %q not found in deploy map", service),
			})
		} else {
			fmt.Fprintf(os.Stderr, "error: service %q not found in deploy map\n", service)
		}
		return 1
	}

	op := rollback.New(svcCfg, service, targetVersion, deps)
	result, err := op.Run(ctx)

	if jsonOutput {
		if err != nil {
			writeJSON(jsonErrorResponse{Success: false, Error: err.Error()})
		} else {
			writeJSON(jsonDeployResponse{
				Success:         true,
				Version:         result.Version,
				PreviousVersion: result.PreviousVersion,
			})
		}
	} else {
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: rollback failed: %v\n", err)
			return 1
		}
		fmt.Printf("Rolled back %s to %s\n", service, result.Version)
	}

	if err != nil {
		return 1
	}
	return 0
}

func runStatus(ctx context.Context, cfg *config.DeployMap, service string, deps status.Deps, jsonOutput bool) int {
	svcCfg, ok := cfg.GetService(service)
	if !ok {
		if jsonOutput {
			writeJSON(jsonErrorResponse{
				Success: false,
				Error:   fmt.Sprintf("service %q not found in deploy map", service),
			})
		} else {
			fmt.Fprintf(os.Stderr, "error: service %q not found in deploy map\n", service)
		}
		return 1
	}

	op := status.New(svcCfg, service, deps)
	result, err := op.Run(ctx)

	if jsonOutput {
		if err != nil {
			writeJSON(jsonErrorResponse{Success: false, Error: err.Error()})
		} else {
			writeJSON(jsonStatusResponse{
				Success:         true,
				Service:         service,
				CurrentVersion:  result.CurrentVersion,
				PreviousVersion: result.PreviousVersion,
				Active:          result.Active,
			})
		}
	} else {
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: status failed: %v\n", err)
			return 1
		}
		fmt.Printf("Service: %s\n", service)
		fmt.Printf("Current: %s\n", result.CurrentVersion)
		if result.PreviousVersion != "" {
			fmt.Printf("Previous: %s\n", result.PreviousVersion)
		}
		fmt.Printf("Active: %t\n", result.Active)
	}

	if err != nil {
		return 1
	}
	return 0
}

func runPrune(ctx context.Context, cfg *config.DeployMap, service string, keep int, deps prune.Deps, jsonOutput bool) int {
	svcCfg, ok := cfg.GetService(service)
	if !ok {
		if jsonOutput {
			writeJSON(jsonErrorResponse{
				Success: false,
				Error:   fmt.Sprintf("service %q not found in deploy map", service),
			})
		} else {
			fmt.Fprintf(os.Stderr, "error: service %q not found in deploy map\n", service)
		}
		return 1
	}

	if keep == 0 {
		keep = svcCfg.KeepReleases
	}

	op := prune.New(svcCfg, service, keep, deps)
	result, err := op.Run(ctx)

	if jsonOutput {
		if err != nil {
			writeJSON(jsonErrorResponse{Success: false, Error: err.Error()})
		} else {
			writeJSON(jsonPruneResponse{
				Success:   true,
				Removed:   len(result.Removed),
				Remaining: result.Remaining,
			})
		}
	} else {
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: prune failed: %v\n", err)
			return 1
		}
		fmt.Printf("Pruned %d releases, %d remaining\n", len(result.Removed), result.Remaining)
		for _, r := range result.Removed {
			fmt.Printf("  - removed: %s\n", r)
		}
	}

	if err != nil {
		return 1
	}
	return 0
}

func printUsage() {
	fmt.Println(`svc-deploy - host-local deployment manager for Go + SQLite services

Usage:
  svc-deploy [global-options] <command> [args]

Global Options:
  -c, --config <path>   Path to deploy-map.toml (default: auto-detect)
  -j, --json            Output results as JSON
  -v, --version         Show version
  -h, --help            Show this help

Commands:
  deploy <service> <version>     Deploy a service version
  rollback <service> [version]   Rollback to previous or specific version
  status <service>               Show service status
  prune <service> [--keep N]     Clean up old releases

Examples:
  svc-deploy deploy svc-a v1.2.3
  svc-deploy rollback svc-a
  svc-deploy status svc-a --json`)
}

func parseInt(s string) int {
	var n int
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}
