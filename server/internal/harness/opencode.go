package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrCLINotFound is returned when neither opencode nor claude CLI is available.
var ErrCLINotFound = errors.New("harness CLI not found")

// ErrRateLimited is a sentinel for 529/rate-limit errors that should be retried with backoff.
var ErrRateLimited = errors.New("harness rate limited (529)")

// OpencodeHarness executes agent jobs via the opencode or claude CLI.
//
// It implements Harness and is the Phase 4 execution engine behind the
// Harness interface. The harness shape is: prompt in, WorkDir, JSON out,
// exit code. Diff is left to the caller/worktree package.
//
// CLI selection: prefer `opencode` if available, fallback to `claude`.
// Detect via exec.LookPath and log the choice. If CLIPath is set explicitly,
// that path is used directly (useful for tests that inject a fake script).
//
// Prompt is derived from spec.SystemPrompt + TaskName + TaskBody.
// WorkDir comes from spec.WorkDir (per-job worktree), falling back to
// h.WorkDir, then os.Getwd. Env is inherited, plus MAX_BUDGET_USD fallback
// for opencode when BudgetUSD is set.
type OpencodeHarness struct {
	CLIPath string
	WorkDir string
}

// Option configures an OpencodeHarness.
type Option func(*OpencodeHarness)

// WithCLIPath overrides the harness binary path (e.g. "/usr/bin/opencode" or a fake script for tests).
func WithCLIPath(p string) Option {
	return func(h *OpencodeHarness) { h.CLIPath = p }
}

// WithWorkDir sets the fallback working directory when spec.WorkDir is empty.
func WithWorkDir(dir string) Option {
	return func(h *OpencodeHarness) { h.WorkDir = dir }
}

// NewOpencodeHarness creates an OpencodeHarness with the given options.
// Example:
//
//	h := harness.NewOpencodeHarness(harness.WithCLIPath("/usr/local/bin/opencode"))
//	h := harness.NewOpencodeHarness() // auto-detects opencode or claude via LookPath
func NewOpencodeHarness(opts ...Option) *OpencodeHarness {
	h := &OpencodeHarness{}
	for _, o := range opts {
		o(h)
	}
	return h
}

// NewOpencodeHarnessWithCLI is a convenience constructor matching the simple
// NewOpencodeHarness(cliPath string) form mentioned in the task spec.
// Prefer NewOpencodeHarness(WithCLIPath(...)) in new code.
func NewOpencodeHarnessWithCLI(cliPath string) *OpencodeHarness {
	return &OpencodeHarness{CLIPath: cliPath}
}

func (h *OpencodeHarness) resolveCLI() (string, string, error) {
	if h.CLIPath != "" {
		kind := detectKind(h.CLIPath)
		return h.CLIPath, kind, nil
	}
	if p, err := exec.LookPath("opencode"); err == nil {
		log.Printf("harness: using opencode at %s", p)
		return p, "opencode", nil
	}
	if p, err := exec.LookPath("claude"); err == nil {
		log.Printf("harness: using claude at %s", p)
		return p, "claude", nil
	}
	return "", "", fmt.Errorf("%w: neither opencode nor claude found in PATH", ErrCLINotFound)
}

func detectKind(cli string) string {
	base := strings.ToLower(filepath.Base(cli))
	if strings.Contains(base, "opencode") {
		return "opencode"
	}
	if strings.Contains(base, "claude") {
		return "claude"
	}
	return "claude"
}

func buildPrompt(spec RunSpec) string {
	var parts []string
	if spec.SystemPrompt != "" {
		parts = append(parts, spec.SystemPrompt)
	}
	if spec.TaskName != "" {
		parts = append(parts, "Task: "+spec.TaskName)
	}
	if spec.TaskBody != "" {
		parts = append(parts, spec.TaskBody)
	}
	if len(parts) == 0 {
		return "hello"
	}
	return strings.Join(parts, "\n\n")
}

func buildArgs(kind string, spec RunSpec, prompt string) []string {
	if kind == "opencode" {
		args := []string{"run", prompt, "--format", "json"}
		if spec.MCPConfigPath != "" {
			// opencode has no --mcp-config flag (yargs rejects it with help exit 1);
			// MCP config is passed via env AYCORN_MCP_CONFIG instead (see Run).
		}
		return args
	}
	args := []string{"-p", prompt, "--output-format", "json", "--bare"}
	budget := budgetValue(spec)
	if budget > 0 {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(budget, 'f', -1, 64))
	}
	if spec.MCPConfigPath != "" {
		args = append(args, "--mcp-config", spec.MCPConfigPath)
	}
	return args
}

func budgetValue(spec RunSpec) float64 {
	if spec.BudgetUSD != nil && *spec.BudgetUSD > 0 {
		return *spec.BudgetUSD
	}
	return 0
}

func workDirFor(h *OpencodeHarness, spec RunSpec) string {
	if spec.WorkDir != "" {
		return spec.WorkDir
	}
	if h.WorkDir != "" {
		return h.WorkDir
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}

// harnessJSON mirrors the spike-noted JSON keys: is_error/result/usage/total_cost_usd/permission_denials
// plus auxiliary fields observed during the spike (subtype, session_id, etc.) and api_error_status
// for 529 detection.
type harnessJSON struct {
	IsError           bool            `json:"is_error"`
	Result            string          `json:"result"`
	TotalCost         float64         `json:"total_cost_usd"`
	Usage             json.RawMessage `json:"usage"`
	PermissionDenials json.RawMessage `json:"permission_denials"`
	Subtype           string          `json:"subtype"`
	StopReason        string          `json:"stop_reason"`
	SessionID         string          `json:"session_id"`
	NumTurns          int             `json:"num_turns"`
	ModelUsage        json.RawMessage `json:"modelUsage"`
	APIErrorStatus    *int            `json:"api_error_status"`
}

// Run executes the harness CLI for spec, capturing stdout/stderr, parsing
// the JSON envelope, and returning a RunResult. It respects ctx cancellation
// and timeout (via exec.CommandContext), captures raw JSON as UsageJson,
// leaves Diff empty (caller/worktree fills it), maps exit codes, and wraps
// 529/rate-limit errors for caller retry. If no CLI is found it returns
// a clear "harness CLI not found" error.
func (h *OpencodeHarness) Run(ctx context.Context, spec RunSpec) (RunResult, error) {
	select {
	case <-ctx.Done():
		return RunResult{}, ctx.Err()
	default:
	}

	cli, kind, err := h.resolveCLI()
	if err != nil {
		return RunResult{}, err
	}

	prompt := buildPrompt(spec)
	args := buildArgs(kind, spec, prompt)
	dir := workDirFor(h, spec)

	cmd := exec.CommandContext(ctx, cli, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if kind == "opencode" {
		if b := budgetValue(spec); b > 0 {
			cmd.Env = append(cmd.Env, fmt.Sprintf("MAX_BUDGET_USD=%g", b))
		}
	}
	if spec.MCPConfigPath != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("AYCORN_MCP_CONFIG=%s", spec.MCPConfigPath))
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("harness: running %s %v in %s (kind=%s)", cli, args, dir, kind)

	runErr := cmd.Run()

	// Respect ctx cancellation — return ctx.Err() directly, do not wrap or swallow.
	if ctx.Err() != nil {
		return RunResult{}, ctx.Err()
	}

	raw := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())
	combined := stdout.String()
	if stderrStr != "" {
		if combined != "" {
			combined += "\n"
		}
		combined += stderrStr
	}
	combined = strings.TrimSpace(combined)

	// If stdout is empty but stderr holds JSON (some CLIs write to stderr), fallback.
	if raw == "" && stderrStr != "" {
		raw = stderrStr
	}
	usageJSON := raw
	if usageJSON == "" {
		usageJSON = combined
	}
	if usageJSON == "" {
		usageJSON = "{}"
	}

	// Detect CLI not found at exec time (e.g. CLIPath points to missing file).
	if runErr != nil && isCLINotFoundErr(runErr) {
		return RunResult{}, fmt.Errorf("harness CLI not found: %w", runErr)
	}

	// Quick rate-limit check on raw text before JSON parse (covers non-JSON 529 pages).
	lowerRaw := strings.ToLower(raw + " " + stderrStr + " " + combined)
	rateLimitHint := strings.Contains(lowerRaw, "529") ||
		strings.Contains(lowerRaw, "rate limit") ||
		strings.Contains(lowerRaw, "rate_limit")

	var parsed harnessJSON
	parseErr := json.Unmarshal([]byte(raw), &parsed)
	// Opencode emits NDJSON (one JSON per line) with --format json, not a single object.
	// If single-object parse fails, try JSONL assembly (opencode) before erroring.
	if parseErr != nil {
		if kind == "opencode" {
			if opResult, opUsage, ok := parseOpencodeJSONL(raw); ok {
				parsed.Result = opResult
				parsed.Usage = json.RawMessage(opUsage)
				parsed.IsError = false
				parseErr = nil
				// Keep raw usage for storage
				usageJSON = raw
				raw = opResult
			} else if combined != raw && combined != "" {
				if opResult2, opUsage2, ok2 := parseOpencodeJSONL(combined); ok2 {
					parsed.Result = opResult2
					parsed.Usage = json.RawMessage(opUsage2)
					parsed.IsError = false
					parseErr = nil
					raw = opResult2
					usageJSON = combined
				}
			}
		}
		if parseErr != nil && combined != raw && combined != "" {
			if json.Unmarshal([]byte(combined), &parsed) == nil {
				parseErr = nil
				raw = combined
				usageJSON = combined
			}
		}
	}

	if parseErr != nil {
		if rateLimitHint {
			exitCode := exitCodeFromState(cmd)
			return RunResult{
				Output:    raw,
				Summary:   truncate(raw, 120),
				ExitCode:  exitCode,
				UsageJson: usageJSON,
			}, fmt.Errorf("harness rate limited (529): %w: %s", ErrRateLimited, raw)
		}
		exitCode := exitCodeFromState(cmd)
		if runErr != nil && exitCode == 0 {
			exitCode = 1
		}
		if raw == "" {
			raw = stderrStr
			if raw == "" {
				raw = runErr.Error()
			}
		}
		return RunResult{
			Output:    raw,
			Summary:   truncate(raw, 120),
			ExitCode:  exitCode,
			UsageJson: usageJSON,
		}, fmt.Errorf("harness failed (exit %d): %w: %s", exitCode, runErrOr(parseErr, runErr), raw)
	}

	// Post-parse rate-limit detection via JSON fields.
	if parsed.APIErrorStatus != nil && *parsed.APIErrorStatus == 529 {
		rateLimitHint = true
	}
	lowerResult := strings.ToLower(parsed.Result)
	if strings.Contains(lowerResult, "529") || strings.Contains(lowerResult, "rate limit") || strings.Contains(lowerResult, "rate_limit") {
		rateLimitHint = true
	}
	if rateLimitHint {
		exitCode := exitCodeFromState(cmd)
		if exitCode == 0 {
			exitCode = 1
		}
		return RunResult{
			Output:    parsed.Result,
			Summary:   truncate(parsed.Result, 120),
			ExitCode:  exitCode,
			UsageJson: usageJSON,
		}, fmt.Errorf("harness rate limited (529): %w: %s", ErrRateLimited, parsed.Result)
	}

	exitCode := exitCodeFromState(cmd)
	isErr := parsed.IsError
	if isErr && exitCode == 0 {
		exitCode = 1
	}
	if runErr != nil && exitCode == 0 {
		exitCode = 1
	}

	if exitCode == 0 && !isErr {
		return RunResult{
			Output:    parsed.Result,
			Summary:   truncate(parsed.Result, 120),
			ExitCode:  0,
			UsageJson: usageJSON,
			Diff:      "",
			Truncated: false,
		}, nil
	}

	return RunResult{
		Output:    parsed.Result,
		Summary:   truncate(parsed.Result, 120),
		ExitCode:  exitCode,
		UsageJson: usageJSON,
		Diff:      "",
		Truncated: false,
	}, fmt.Errorf("harness failed (exit %d, is_error=%v): %s", exitCode, isErr, parsed.Result)
}

func exitCodeFromState(cmd *exec.Cmd) int {
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}
	return 1
}

func runErrOr(a, b error) error {
	if b != nil {
		return b
	}
	return a
}

func isCLINotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "no such file or directory") ||
		strings.Contains(msg, "harness CLI not found")
}

func parseOpencodeJSONL(raw string) (string, string, bool) {
	lines := strings.Split(raw, "\n")
	var out strings.Builder
	var usage string
	found := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var evt map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		if tRaw, ok := evt["type"]; ok {
			var typ string
			if err := json.Unmarshal(tRaw, &typ); err != nil {
				continue
			}
			if typ == "text" {
				var part struct {
					Text string `json:"text"`
				}
				if pRaw, ok := evt["part"]; ok {
					_ = json.Unmarshal(pRaw, &part)
					if part.Text != "" {
						if out.Len() > 0 {
							out.WriteString("\n")
						}
						out.WriteString(part.Text)
						found = true
					}
				} else {
					var txt string
					if err := json.Unmarshal(tRaw, &txt); err == nil && txt != "" {
						if out.Len() > 0 {
							out.WriteString("\n")
						}
						out.WriteString(txt)
						found = true
					}
				}
			}
			if typ == "step_finish" {
				usage = line
			}
		}
	}
	if !found {
		return "", "", false
	}
	if usage == "" {
		usage = raw
	}
	return out.String(), usage, true
}
