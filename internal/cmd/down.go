package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/daemon"
	"github.com/steveyegge/gastown/internal/events"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/tmux"
	"github.com/steveyegge/gastown/internal/workspace"
)

var downCmd = &cobra.Command{
	Use:     "down",
	GroupID: GroupServices,
	Short:   "Stop all Gas Town services",
	Long: `Stop all Gas Town long-lived services.

This gracefully shuts down all infrastructure agents:

  • Refineries - Per-rig work processors
  • Witnesses  - Per-rig polecat managers
  • Mayor      - Global work coordinator
  • Boot       - Deacon's watchdog
  • Deacon     - Health orchestrator
  • Daemon     - Go background process

With --all, also stops resurrection layer (bd daemon/activity) and verifies
shutdown. Polecats are NOT stopped - use 'gt swarm stop' for that.

Flags:
  --all      Stop bd daemons/activity, verify complete shutdown
  --nuke     Kill entire tmux server (DESTRUCTIVE!)
  --dry-run  Preview what would be stopped
  --force    Skip graceful shutdown, use SIGKILL`,
	RunE: runDown,
}

var (
	downQuiet  bool
	downForce  bool
	downAll    bool
	downNuke   bool
	downDryRun bool
)

func init() {
	downCmd.Flags().BoolVarP(&downQuiet, "quiet", "q", false, "Only show errors")
	downCmd.Flags().BoolVarP(&downForce, "force", "f", false, "Force kill without graceful shutdown")
	downCmd.Flags().BoolVarP(&downAll, "all", "a", false, "Stop bd daemons/activity and verify shutdown")
	downCmd.Flags().BoolVar(&downNuke, "nuke", false, "Kill entire tmux server (DESTRUCTIVE - kills non-GT sessions!)")
	downCmd.Flags().BoolVar(&downDryRun, "dry-run", false, "Preview what would be stopped without taking action")
	rootCmd.AddCommand(downCmd)
}

func runDown(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	t := tmux.NewTmux()
	allOK := true

	if downDryRun {
		fmt.Println("═══ DRY RUN: Preview of shutdown actions ═══")
		fmt.Println()
	}

	// Phase 1: Stop bd resurrection layer (--all only)
	if downAll {
		daemonsKilled, activityKilled, err := beads.StopAllBdProcesses(downDryRun, downForce)
		if err != nil {
			printDownStatus("bd processes", false, err.Error())
			allOK = false
		} else {
			if downDryRun {
				if daemonsKilled > 0 || activityKilled > 0 {
					printDownStatus("bd daemon", true, fmt.Sprintf("%d would stop", daemonsKilled))
					printDownStatus("bd activity", true, fmt.Sprintf("%d would stop", activityKilled))
				} else {
					printDownStatus("bd processes", true, "none running")
				}
			} else {
				if daemonsKilled > 0 {
					printDownStatus("bd daemon", true, fmt.Sprintf("%d stopped", daemonsKilled))
				}
				if activityKilled > 0 {
					printDownStatus("bd activity", true, fmt.Sprintf("%d stopped", activityKilled))
				}
				if daemonsKilled == 0 && activityKilled == 0 {
					printDownStatus("bd processes", true, "none running")
				}
			}
		}
	}

	rigs := discoverRigs(townRoot)

	// Phase 2a: Stop refineries
	for _, rigName := range rigs {
		sessionName := fmt.Sprintf("gt-%s-refinery", rigName)
		if downDryRun {
			if running, _ := t.HasSession(sessionName); running {
				printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "would stop")
			}
			continue
		}
		if err := stopSession(t, sessionName); err != nil {
			printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), false, err.Error())
			allOK = false
		} else {
			printDownStatus(fmt.Sprintf("Refinery (%s)", rigName), true, "stopped")
		}
	}

	// Phase 2b: Stop witnesses
	for _, rigName := range rigs {
		sessionName := fmt.Sprintf("gt-%s-witness", rigName)
		if downDryRun {
			if running, _ := t.HasSession(sessionName); running {
				printDownStatus(fmt.Sprintf("Witness (%s)", rigName), true, "would stop")
			}
			continue
		}
		if err := stopSession(t, sessionName); err != nil {
			printDownStatus(fmt.Sprintf("Witness (%s)", rigName), false, err.Error())
			allOK = false
		} else {
			printDownStatus(fmt.Sprintf("Witness (%s)", rigName), true, "stopped")
		}
	}

	// Phase 3: Stop town-level sessions (Mayor, Boot, Deacon)
	for _, ts := range session.TownSessions() {
		if downDryRun {
			if running, _ := t.HasSession(ts.SessionID); running {
				printDownStatus(ts.Name, true, "would stop")
			}
			continue
		}
		stopped, err := session.StopTownSession(t, ts, downForce)
		if err != nil {
			printDownStatus(ts.Name, false, err.Error())
			allOK = false
		} else if stopped {
			printDownStatus(ts.Name, true, "stopped")
		} else {
			printDownStatus(ts.Name, true, "not running")
		}
	}

	// Phase 4: Stop Daemon
	running, pid, _ := daemon.IsRunning(townRoot)
	if downDryRun {
		if running {
			printDownStatus("Daemon", true, fmt.Sprintf("would stop (PID %d)", pid))
		}
	} else {
		if running {
			if err := daemon.StopDaemon(townRoot); err != nil {
				printDownStatus("Daemon", false, err.Error())
				allOK = false
			} else {
				printDownStatus("Daemon", true, fmt.Sprintf("stopped (was PID %d)", pid))
			}
		} else {
			printDownStatus("Daemon", true, "not running")
		}
	}

	// Phase 5: Nuke tmux server (--nuke only, DESTRUCTIVE)
	if downNuke {
		if !downDryRun && os.Getenv("GT_NUKE_ACKNOWLEDGED") == "" {
			fmt.Println()
			fmt.Printf("%s The --nuke flag kills ALL tmux sessions, not just Gas Town.\n",
				style.Bold.Render("⚠ WARNING:"))
			fmt.Printf("This includes vim sessions, running builds, SSH connections, etc.\n")
			fmt.Printf("Set GT_NUKE_ACKNOWLEDGED=1 to suppress this warning.\n")
			fmt.Println()
		}

		if downDryRun {
			printDownStatus("Tmux server", true, "would kill (DESTRUCTIVE)")
		} else {
			if err := t.KillServer(); err != nil {
				printDownStatus("Tmux server", false, err.Error())
				allOK = false
			} else {
				printDownStatus("Tmux server", true, "killed (all tmux sessions destroyed)")
			}
		}
	}

	// Summary
	fmt.Println()
	if downDryRun {
		fmt.Println("═══ DRY RUN COMPLETE (no changes made) ═══")
		return nil
	}

	if allOK {
		fmt.Printf("%s All services stopped\n", style.Bold.Render("✓"))
		stoppedServices := []string{"daemon", "deacon", "boot", "mayor"}
		for _, rigName := range rigs {
			stoppedServices = append(stoppedServices, fmt.Sprintf("%s/refinery", rigName))
			stoppedServices = append(stoppedServices, fmt.Sprintf("%s/witness", rigName))
		}
		if downAll {
			stoppedServices = append(stoppedServices, "bd-processes")
		}
		if downNuke {
			stoppedServices = append(stoppedServices, "tmux-server")
		}
		_ = events.LogFeed(events.TypeHalt, "gt", events.HaltPayload(stoppedServices))
	} else {
		fmt.Printf("%s Some services failed to stop\n", style.Bold.Render("✗"))
		return fmt.Errorf("not all services stopped")
	}

	return nil
}

func printDownStatus(name string, ok bool, detail string) {
	if downQuiet && ok {
		return
	}
	if ok {
		fmt.Printf("%s %s: %s\n", style.SuccessPrefix, name, style.Dim.Render(detail))
	} else {
		fmt.Printf("%s %s: %s\n", style.ErrorPrefix, name, detail)
	}
}

// stopSession gracefully stops a tmux session.
func stopSession(t *tmux.Tmux, sessionName string) error {
	running, err := t.HasSession(sessionName)
	if err != nil {
		return err
	}
	if !running {
		return nil // Already stopped
	}

	// Try graceful shutdown first (Ctrl-C, best-effort interrupt)
	if !downForce {
		_ = t.SendKeysRaw(sessionName, "C-c")
		time.Sleep(100 * time.Millisecond)
	}

	// Kill the session
	return t.KillSession(sessionName)
}
