// Command eval runs opt-in, real-model evaluations against disposable fixtures.
package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"github.com/waseem-polus/aycorn/server/internal/evals"
	"os"
	"os/exec"
	"os/signal"
	"strings"
)

func main() {
	var o evals.Options
	flag.StringVar(&o.Model, "model", "gpt-5.6-sol", "installed Codex model")
	flag.StringVar(&o.Executable, "codex", "", "Codex executable (auto-detect by default)")
	flag.StringVar(&o.MCP, "mcp", "./aycorn-mcp", "built Aycorn MCP executable")
	flag.StringVar(&o.Python, "python", "python3", "Python 3 used by deterministic graders")
	flag.StringVar(&o.Output, "output", "../evals/results/latest", "JSON/CSV path prefix")
	flag.IntVar(&o.Repeat, "repeat", 1, "trials per case and variant")
	flag.IntVar(&o.TimeoutSeconds, "timeout", 120, "seconds per agent turn")
	variant := flag.String("variant", "both", "baseline, verify-first, or both (paired)")
	list := flag.Bool("list", false, "list fixed cases without invoking a model")
	live := flag.Bool("live", false, "run real model turns using the local Codex login")
	flag.Parse()
	if *list {
		s, err := evals.Load()
		if err != nil {
			fatal(err)
		}
		for _, c := range s.Cases {
			fmt.Printf("%s\t%s\n", c.ID, c.Category)
		}
		return
	}
	if !*live {
		fatal(fmt.Errorf("real runs require -live; use -list to inspect the dataset without model usage"))
	}
	if *variant == "both" {
		o.Variants = []string{"baseline", "verify-first"}
	} else {
		o.Variants = []string{*variant}
	}
	if raw, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		o.SourceCommit = strings.TrimSpace(string(raw))
	}
	if raw, err := exec.Command("git", "diff", "HEAD", "--").Output(); err == nil {
		hash := sha256.Sum256(raw)
		o.SourceDiffHash = fmt.Sprintf("%x", hash)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	r, err := evals.Run(ctx, o, os.Stdout)
	if err != nil {
		fatal(err)
	}
	for name, s := range r.Summary {
		fmt.Printf("%s: %d/%d solved (%.1f%%), %d tokens reported\n", name, s.Passed, s.Completed, s.SolveRate*100, s.Tokens)
	}
	if r.DeltaPoints != nil {
		fmt.Printf("Verify-first minus baseline: %+.1f percentage points\n", *r.DeltaPoints)
	}
	fmt.Printf("Results: %s.json and %s.csv\n", o.Output, o.Output)
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
