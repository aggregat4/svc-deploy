package main

import (
	"context"
	"fmt"
	"os"
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

// parseGlobalFlags extracts global flags and returns remaining args.
// It handles flags in any position before or after the command.
func parseGlobalFlags(args []string) (cliFlags, []string, error) {
	var flags cliFlags
	var remaining []string

	i := 0
	for i < len(args) {
		arg := args[i]

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
		case "--":
			// End of flags, rest are positional
			remaining = append(remaining, args[i+1:]...)
			return flags, remaining, nil
		default:
			if len(arg) > 0 && arg[0] == '-' {
				return flags, nil, fmt.Errorf("unknown flag: %s", arg)
			}
			// Positional argument
			remaining = append(remaining, arg)
			i++
		}
	}

	return flags, remaining, nil
}

func main() {
	// Parse global flags from all args (before and after command)
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
			fmt.Printf(`{"success":false,"error":"loading config: %s"}`+"\n", err)
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
			fmt.Printf(`{"success":false,"error":"service %q not found in deploy map"}\n`, service)
		} else {
			fmt.Fprintf(os.Stderr, "error: service %q not found in deploy map\n", service)
		}
		return 1
	}

	op := deploy.New(svcCfg, service, version, deps)
	result, err := op.Run(ctx)

	if jsonOutput {
		if err != nil {
			fmt.Printf(`{"success":false,"error":%q}\n`, err.Error())
		} else {
			fmt.Printf(`{"success":true,"version":%q,"previous_version":%q}\n`, result.Version, result.PreviousVersion)
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
			fmt.Printf(`{"success":false,"error":"service %q not found in deploy map"}\n`, service)
		} else {
			fmt.Fprintf(os.Stderr, "error: service %q not found in deploy map\n", service)
		}
		return 1
	}

	op := rollback.New(svcCfg, service, targetVersion, deps)
	result, err := op.Run(ctx)

	if jsonOutput {
		if err != nil {
			fmt.Printf(`{"success":false,"error":%q}\n`, err.Error())
		} else {
			fmt.Printf(`{"success":true,"version":%q,"previous_version":%q}\n`, result.Version, result.PreviousVersion)
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
			fmt.Printf(`{"success":false,"error":"service %q not found in deploy map"}\n`, service)
		} else {
			fmt.Fprintf(os.Stderr, "error: service %q not found in deploy map\n", service)
		}
		return 1
	}

	op := status.New(svcCfg, service, deps)
	result, err := op.Run(ctx)

	if jsonOutput {
		if err != nil {
			fmt.Printf(`{"success":false,"error":%q}\n`, err.Error())
		} else {
			fmt.Printf(`{"success":true,"service":%q,"current_version":%q,"previous_version":%q,"active":%t}\n`,
				service, result.CurrentVersion, result.PreviousVersion, result.Active)
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
			fmt.Printf(`{"success":false,"error":"service %q not found in deploy map"}\n`, service)
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
			fmt.Printf(`{"success":false,"error":%q}\n`, err.Error())
		} else {
			fmt.Printf(`{"success":true,"removed":%d,"remaining":%d}\n`, len(result.Removed), result.Remaining)
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
