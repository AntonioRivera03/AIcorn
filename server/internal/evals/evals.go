// Package evals benchmarks the production agent adapter against isolated fixtures.
package evals

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/waseem-polus/aycorn/server/internal/appdb"
	"github.com/waseem-polus/aycorn/server/internal/harness"
	"github.com/waseem-polus/aycorn/server/internal/models"
	_ "modernc.org/sqlite"
)

//go:embed cases.json
var dataset []byte

type Check struct {
	Input    any `json:"input"`
	Expected any `json:"expected"`
}
type Case struct {
	ID       string  `json:"id"`
	Category string  `json:"category"`
	Prompt   string  `json:"prompt"`
	Starter  string  `json:"starter,omitempty"`
	Checks   []Check `json:"checks,omitempty"`
	Grader   string  `json:"grader,omitempty"`
}
type Suite struct {
	Version int    `json:"version"`
	Cases   []Case `json:"cases"`
}

func Load() (Suite, error) {
	var s Suite
	if err := json.Unmarshal(dataset, &s); err != nil {
		return s, err
	}
	seen := map[string]bool{}
	for _, c := range s.Cases {
		if c.ID == "" || seen[c.ID] || c.Prompt == "" || (c.Category == "coding" && len(c.Checks) == 0) || (c.Category != "coding" && c.Grader == "") {
			return s, fmt.Errorf("invalid case %q", c.ID)
		}
		seen[c.ID] = true
	}
	return s, nil
}

var Prompts = map[string]string{
	"baseline":     "Complete the assigned task. Use the provided workspace and Aycorn tools. Do not delegate. Keep the final response concise.",
	"verify-first": "Complete the assigned task. Do not delegate. Before acting, read the current ticket through Aycorn MCP and identify the exact acceptance conditions. For code, check empty inputs, boundaries, and preservation of existing data, then run a local check before answering. For ticket operations, verify the final state through the available tools and avoid duplicate references. Keep the final response concise.",
}

type Options struct {
	Model, Executable, MCP, Python, Output, SourceCommit, SourceDiffHash string
	Variants                                                             []string
	Repeat, TimeoutSeconds                                               int
}
type Tokens struct {
	Input       int `json:"input"`
	CachedInput int `json:"cachedInput"`
	Output      int `json:"output"`
	Total       int `json:"total"`
}
type Result struct {
	Case      string          `json:"case"`
	Category  string          `json:"category"`
	Variant   string          `json:"variant"`
	Trial     int             `json:"trial"`
	Passed    bool            `json:"passed"`
	LatencyMS int64           `json:"latencyMs"`
	Grade     string          `json:"grade"`
	Error     string          `json:"error,omitempty"`
	Tokens    *Tokens         `json:"tokens"`
	CostUSD   *float64        `json:"costUsd"`
	Usage     json.RawMessage `json:"usage"`
	Output    string          `json:"output"`
}
type Summary struct {
	Completed     int     `json:"completed"`
	Passed        int     `json:"passed"`
	SolveRate     float64 `json:"solveRate"`
	LatencyMS     int64   `json:"latencyMs"`
	Tokens        int     `json:"tokens"`
	UsageReported int     `json:"usageReported"`
}
type Report struct {
	SchemaVersion    int                `json:"schemaVersion"`
	SuiteVersion     int                `json:"suiteVersion"`
	SuiteSHA256      string             `json:"suiteSha256"`
	SourceCommit     string             `json:"sourceCommit"`
	SourceDiffSHA256 string             `json:"sourceDiffSha256"`
	Model            string             `json:"model"`
	EngineVersion    string             `json:"engineVersion"`
	StartedAt        string             `json:"startedAt"`
	CompletedAt      string             `json:"completedAt,omitempty"`
	Planned          int                `json:"planned"`
	Prompts          map[string]string  `json:"prompts"`
	Results          []Result           `json:"results"`
	Summary          map[string]Summary `json:"summary"`
	DeltaPoints      *float64           `json:"solveRateDeltaPoints,omitempty"`
	CostNote         string             `json:"costNote"`
}

func (r *Report) summarize() {
	r.Summary = map[string]Summary{}
	for _, result := range r.Results {
		s := r.Summary[result.Variant]
		s.Completed++
		if result.Passed {
			s.Passed++
		}
		s.LatencyMS += result.LatencyMS
		if result.Tokens != nil {
			s.Tokens += result.Tokens.Total
			s.UsageReported++
		}
		s.SolveRate = float64(s.Passed) / float64(s.Completed)
		r.Summary[result.Variant] = s
	}
	before, b := r.Summary["baseline"]
	after, a := r.Summary["verify-first"]
	r.DeltaPoints = nil
	if b && a && before.Completed == after.Completed {
		d := (after.SolveRate - before.SolveRate) * 100
		r.DeltaPoints = &d
	}
}
func usageTokens(raw string) *Tokens {
	var u struct {
		Total *struct {
			Input  int `json:"inputTokens"`
			Cached int `json:"cachedInputTokens"`
			Output int `json:"outputTokens"`
			Total  int `json:"totalTokens"`
		} `json:"total"`
	}
	if json.Unmarshal([]byte(raw), &u) != nil || u.Total == nil {
		return nil
	}
	return &Tokens{Input: u.Total.Input, CachedInput: u.Total.Cached, Output: u.Total.Output, Total: u.Total.Total}
}
func Run(ctx context.Context, o Options, progress io.Writer) (*Report, error) {
	suite, err := Load()
	if err != nil {
		return nil, err
	}
	if o.Repeat < 1 || o.TimeoutSeconds < 1 || len(o.Variants) == 0 {
		return nil, errors.New("positive repeat/timeout and at least one variant are required")
	}
	prompts := map[string]string{}
	for _, v := range o.Variants {
		p, ok := Prompts[v]
		if !ok || prompts[v] != "" {
			return nil, fmt.Errorf("unknown or duplicate variant %q", v)
		}
		prompts[v] = p
	}
	health := harness.CheckEngine(ctx, o.Executable)
	if !health.Ready {
		return nil, errors.New(health.Error)
	}
	if !models.IsOpenAIModel(o.Model) {
		return nil, errors.New("select an OpenAI model supported by the installed Codex")
	}
	if o.MCP, err = filepath.Abs(o.MCP); err != nil {
		return nil, err
	}
	if _, err = os.Stat(o.MCP); err != nil {
		return nil, err
	}
	if o.Python, err = exec.LookPath(o.Python); err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "aycorn-eval-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(root)
	hash := sha256.Sum256(dataset)
	report := &Report{SchemaVersion: 1, SuiteVersion: suite.Version, SuiteSHA256: fmt.Sprintf("%x", hash), SourceCommit: o.SourceCommit, SourceDiffSHA256: o.SourceDiffHash, Model: o.Model, EngineVersion: health.Version, StartedAt: time.Now().UTC().Format(time.RFC3339), Planned: len(suite.Cases) * o.Repeat * len(o.Variants), Prompts: prompts, Results: []Result{}, CostNote: "Native Codex reports token usage, not attributable dollar cost. costUsd=null means unavailable; subscription usage is not an API price estimate."}
	if err = Save(o.Output, report); err != nil {
		return report, err
	}
	for trial := 1; trial <= o.Repeat; trial++ {
		for _, c := range suite.Cases {
			for _, v := range o.Variants {
				if ctx.Err() != nil {
					return report, ctx.Err()
				}
				fmt.Fprintf(progress, "%s %s trial %d…\n", v, c.ID, trial)
				result := runCase(ctx, root, health.Executable, o, c, v, trial)
				report.Results = append(report.Results, result)
				if err = Save(o.Output, report); err != nil {
					return report, err
				}
				fmt.Fprintf(progress, "  passed=%t latency=%dms %s\n", result.Passed, result.LatencyMS, result.Grade)
			}
		}
	}
	report.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	return report, Save(o.Output, report)
}
func runCase(ctx context.Context, root, executable string, o Options, c Case, variant string, trial int) Result {
	r := Result{Case: c.ID, Category: c.Category, Variant: variant, Trial: trial, Usage: json.RawMessage(`{}`)}
	started := time.Now()
	dir := filepath.Join(root, fmt.Sprintf("%s-%s-%d", c.ID, variant, trial))
	work := filepath.Join(dir, "workspace")
	fail := func(err error) Result {
		r.Error = err.Error()
		r.Grade = "setup or agent failure"
		r.LatencyMS = time.Since(started).Milliseconds()
		return r
	}
	if err := os.MkdirAll(work, 0700); err != nil {
		return fail(err)
	}
	if c.Category == "coding" {
		if err := os.WriteFile(filepath.Join(work, "solution.py"), []byte(c.Starter), 0600); err != nil {
			return fail(err)
		}
	}
	dbPath := filepath.Join(dir, "fixture.db")
	db, err := appdb.Open(dbPath)
	if err != nil {
		return fail(err)
	}
	defer db.Close()
	if err = appdb.Migrate(db, dbPath); err != nil {
		return fail(err)
	}
	if err = seed(db, c); err != nil {
		return fail(err)
	}
	req := &models.AIRunRequest{Engine: "codex", Intent: "ask", ProjectID: 101, TaskName: "Eval " + c.ID, TaskBody: c.Prompt, Instruction: c.Prompt, SystemPrompt: Prompts[variant], Model: o.Model, Executable: executable, TimeoutSeconds: o.TimeoutSeconds}
	if c.Category == "coding" {
		req.Intent = "implement"
		req.RepoPath = work
	}
	engine := &harness.Codex{MCPExecutable: o.MCP, DBPath: dbPath, FleetDir: filepath.Join(dir, "fleet")}
	output, runErr := engine.Run(ctx, harness.RunSpec{Request: req, TaskID: 101, JobID: 101, WorkDir: work})
	r.LatencyMS = time.Since(started).Milliseconds()
	r.Output = strings.ReplaceAll(output.Output, root, "<eval-workspace>")
	if json.Valid([]byte(output.UsageJson)) {
		r.Usage = json.RawMessage(output.UsageJson)
	}
	r.Tokens = usageTokens(output.UsageJson)
	if runErr != nil {
		r.Error = strings.ReplaceAll(runErr.Error(), root, "<eval-workspace>")
	}
	gradeErr := grade(ctx, o.Python, work, db, c, output.Output)
	if gradeErr != nil {
		r.Grade = gradeErr.Error()
	} else if runErr != nil {
		r.Grade = "postconditions passed but agent run failed"
	} else {
		r.Grade = "all postconditions passed"
		r.Passed = true
	}
	return r
}
func seed(db *sql.DB, c Case) error {
	_, err := db.Exec(`INSERT INTO workflow(id,name) VALUES(101,'Eval');
 INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(101,101,'Open','open','gray','circle',1);
 INSERT INTO project(id,workflow,name) VALUES(101,101,'Disposable eval');
 INSERT INTO checklist(id,project,name) VALUES(101,101,'Eval');`)
	if err != nil {
		return err
	}
	body, _ := json.Marshal([]any{map[string]any{"type": "p", "children": []any{map[string]string{"text": c.Prompt}}}})
	if _, err = db.Exec("INSERT INTO task(id,checklist,stage,type,name,priority,body) VALUES(101,101,101,1,?,'High',?)", "Eval "+c.ID, string(body)); err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO agent_job(id,task,status,requestJson) VALUES(101,101,'running','{"engine":"codex","projectId":101}')`)
	return err
}

const graderScript = `import importlib.util,json,sys
spec=importlib.util.spec_from_file_location("candidate",sys.argv[1])
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
checks=json.load(sys.stdin)
for i,c in enumerate(checks):
 actual=m.solve(c["input"])
 if actual != c["expected"]: raise AssertionError("case %d: expected %r, got %r" % (i,c["expected"],actual))
print("passed %d checks" % len(checks))`

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 65536 {
		keep := min(n, 65536-b.Len())
		b.Buffer.Write(p[:keep])
	}
	return n, nil
}
func grade(ctx context.Context, python, work string, db *sql.DB, c Case, output string) error {
	if c.Category == "coding" {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		payload, _ := json.Marshal(c.Checks)
		cmd := exec.CommandContext(ctx, python, "-I", "-c", graderScript, filepath.Join(work, "solution.py"))
		cmd.Dir = work
		cmd.Stdin = bytes.NewReader(payload)
		var logs boundedOutput
		cmd.Stdout = &logs
		cmd.Stderr = &logs
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("grader: %v: %s", err, strings.TrimSpace(logs.String()))
		}
		return nil
	}
	var title, priority, body string
	var stage int
	if err := db.QueryRow("SELECT name,priority,body,stage FROM task WHERE id=101").Scan(&title, &priority, &body, &stage); err != nil {
		return err
	}
	expectedBody, _ := json.Marshal([]any{map[string]any{"type": "p", "children": []any{map[string]string{"text": c.Prompt}}}})
	if title != "Eval "+c.ID || priority != "High" || stage != 101 || body != string(expectedBody) {
		return errors.New("unrequested task fields changed")
	}
	if c.Grader == "read" {
		var got struct {
			Title    string `json:"title"`
			Priority string `json:"priority"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(output)), &got) != nil || got.Title != title || got.Priority != priority {
			return errors.New("response did not contain the exact ticket title and priority as JSON")
		}
		return nil
	}
	want := "https://github.com/antoniorivera03/aicorn/pull/16"
	label := "Enhancements"
	kind := "pull_request"
	if c.Grader == "branch" {
		want = "https://github.com/antoniorivera03/aicorn/tree/enhancements"
		label = "Enhancements branch"
		kind = "branch"
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM task_github_link").Scan(&count); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("expected one saved link, got %d", count)
	}
	var url, gotLabel, gotKind string
	var task int
	if err := db.QueryRow("SELECT task,url,label,kind FROM task_github_link").Scan(&task, &url, &gotLabel, &gotKind); err != nil {
		return err
	}
	if task != 101 || url != want || gotLabel != label || gotKind != kind {
		return fmt.Errorf("wrong saved reference: %d %s %s %s", task, url, gotLabel, gotKind)
	}
	return nil
}
func Save(prefix string, r *Report) error {
	r.summarize()
	if err := os.MkdirAll(filepath.Dir(prefix), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err = atomicFile(prefix+".json", append(raw, '\n')); err != nil {
		return err
	}
	var data bytes.Buffer
	csvWriter := csv.NewWriter(&data)
	csvWriter.Write([]string{"case", "category", "variant", "trial", "passed", "latency_ms", "input_tokens", "cached_input_tokens", "output_tokens", "total_tokens", "cost_usd", "grade", "error"})
	for _, item := range r.Results {
		tokens := []string{"", "", "", ""}
		if t := item.Tokens; t != nil {
			tokens = []string{strconv.Itoa(t.Input), strconv.Itoa(t.CachedInput), strconv.Itoa(t.Output), strconv.Itoa(t.Total)}
		}
		row := []string{item.Case, item.Category, item.Variant, strconv.Itoa(item.Trial), strconv.FormatBool(item.Passed), strconv.FormatInt(item.LatencyMS, 10)}
		row = append(row, tokens...)
		row = append(row, "", item.Grade, item.Error)
		csvWriter.Write(row)
	}
	csvWriter.Flush()
	if err = csvWriter.Error(); err != nil {
		return err
	}
	return atomicFile(prefix+".csv", data.Bytes())
}
func atomicFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".eval-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
