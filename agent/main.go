// Command ipcheck verifies that traffic to each domain group leaves through
// the expected exit IP.
//
//	ipcheck [run]   — background agent: tray icon, notifications, dashboard + API
//	ipcheck status  — print current state (from the running agent, or check directly)
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ipcheck/internal/agent"
	"ipcheck/internal/checker"
	"ipcheck/internal/config"
	"ipcheck/internal/hass"
	"ipcheck/internal/tray"
	"ipcheck/internal/web"
)

func main() {
	cmd := "run"
	args := os.Args[1:]
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}

	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "path to config.yaml")
	noTray := fs.Bool("no-tray", false, "run: headless, without the tray icon")
	asJSON := fs.Bool("json", false, "status: print raw JSON")
	local := fs.Bool("local", false, "status: check directly, do not ask the running agent")
	fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	switch cmd {
	case "run":
		run(cfg, *noTray)
	case "status":
		os.Exit(status(cfg, *asJSON, *local))
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q (use run or status)\n", cmd)
		os.Exit(2)
	}
}

func run(cfg *config.Config, noTray bool) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := agent.New(cfg)
	go a.Run(ctx)

	if cfg.MQTT != nil {
		pub := hass.New(cfg, a)
		pub.Start()
		defer pub.Stop()
	}

	srv := &http.Server{Addr: cfg.Listen, Handler: web.Handler(a), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("dashboard: %s", dashboardURL(cfg))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()

	if noTray {
		<-ctx.Done()
	} else {
		go func() {
			<-ctx.Done()
			tray.Quit()
		}()
		tray.Run(a, len(cfg.Groups), dashboardURL(cfg), stop)
	}

	shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	srv.Shutdown(shutdown)
}

func status(cfg *config.Config, asJSON, local bool) int {
	var s agent.State
	if local || fetchState(cfg, &s) != nil {
		results := checker.New(cfg).CheckAll(context.Background())
		s = agent.MakeState(results, time.Now(), cfg.Interval)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(s)
	} else {
		printState(s)
	}
	if s.OK {
		return 0
	}
	return 1
}

func fetchState(cfg *config.Config, s *agent.State) error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(dashboardURL(cfg) + "api/status")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(s); err != nil {
		return err
	}
	if len(s.Results) == 0 {
		return fmt.Errorf("agent has no results yet")
	}
	return nil
}

func printState(s agent.State) {
	const (
		green = "\033[32m"
		red   = "\033[31m"
		dim   = "\033[2m"
		reset = "\033[0m"
	)
	for _, r := range s.Results {
		col, mark := green, "✔"
		if r.Status != checker.OK {
			col, mark = red, "✘"
		}
		flag := "❔"
		if r.Country != "" {
			flag = checker.Flag(r.Country)
		}
		exit := r.Exit
		if exit == "" {
			exit = "?"
		}
		ip := r.IP
		if ip == "" {
			ip = "—"
		}
		fmt.Printf("%s%s%s %s %-14s %-16s %-14s %s%s · %dms%s\n",
			col, mark, reset, flag, r.Group, ip, exit, dim, r.Host, r.LatencyMs, reset)
		if r.Status != checker.OK {
			fmt.Printf("    %s%s%s\n", col, r.Message, reset)
		}
	}
	if s.OK {
		fmt.Printf("\n%s🟢 все маршруты в порядке%s\n", green, reset)
	} else {
		fmt.Printf("\n%s🔴 проблем: %d%s\n", red, s.Problems, reset)
	}
}

func dashboardURL(cfg *config.Config) string {
	host, port, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return "http://" + cfg.Listen + "/"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/"
}
