package environments

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in acceptance test: uses only newly created, installation-labelled
// namespaces and images in the explicitly named context. Never changes kubeconfig.
// A NetworkPolicy-capable CNI/controller and default StorageClass are required.
func TestKubernetesLive(t *testing.T) {
	if os.Getenv("AYCORN_K8S_LIVE") != "1" {
		t.Skip("set AYCORN_K8S_LIVE=1 and AYCORN_K8S_CONTEXT to run against a disposable kind cluster")
	}
	cluster := os.Getenv("AYCORN_K8S_CLUSTER")
	clusterContext := os.Getenv("AYCORN_K8S_CONTEXT")
	if cluster == "" || clusterContext != "kind-"+cluster {
		t.Fatal("explicit matching kind cluster/context required")
	}
	store := testStore(t)
	server := `const http = require('node:http'); const fs = require('node:fs');
const version = 'one';
http.createServer((req,res) => {
  if(req.url === '/set') fs.writeFileSync('/data/value', version);
  res.setHeader('Content-Type','application/json');
  res.end(JSON.stringify({version, value:fs.existsSync('/data/value') ? fs.readFileSync('/data/value','utf8') : ''}));
}).listen(8000,'0.0.0.0');`
	root := repoFixture(t, map[string]string{"server.cjs": server, "math.cjs": "module.exports = (a,b) => a+b;", "math.test.cjs": "const {test}=require('node:test');const assert=require('node:assert/strict');test('addition',()=>assert.equal(require('./math.cjs')(2,3),5));"})
	configureFixture(t, store, root)
	settings := Defaults()
	settings.Context = clusterContext
	settings.KindCluster = cluster
	settings.Profile = "custom"
	settings.MaxRunning = 3
	settings.TimeoutSeconds = 600
	settings.Dockerfile = "FROM node:24.13.0-bookworm-slim AS test\nWORKDIR /app\nCOPY --chown=1000:1000 . .\nFROM test AS preview\nUSER 1000:1000\nCMD [\"node\",\"/app/server.cjs\"]\n"
	settings.TestCommand = []string{"node", "--test", "/app/math.test.cjs"}
	settings.TestMemory = 512
	raw, _ := json.Marshal(settings)
	if _, err := store.DB.Exec(`UPDATE environment_settings SET settings=? WHERE project=1`, string(raw)); err != nil {
		t.Fatal(err)
	}
	token, _ := store.Installation()
	runtime := &Kubernetes{Token: token, MainURL: "http://127.0.0.1:8000"}
	s := &Service{Store: store, Runtime: runtime, Root: t.TempDir(), builds: make(chan struct{}, 1)}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	t.Cleanup(func() {
		runtime.Close()
		rows, _ := store.List(0, 0, false)
		for i := range rows {
			cleanup, stop := context.WithTimeout(context.Background(), 90*time.Second)
			if err := runtime.Destroy(cleanup, &rows[i]); err != nil {
				t.Errorf("cleanup environment %d: %v", rows[i].ID, err)
			}
			stop()
		}
	})
	start := func(input CreateInput) *Environment {
		t.Helper()
		e, err := s.Create(ctx, 1, input)
		if err != nil {
			t.Fatal(err)
		}
		s.reconcile(ctx, e)
		current, _ := store.Get(e.ID)
		if current.State != "ready" {
			logs, _ := store.Logs(e.ID)
			live, _ := runtime.Logs(ctx, current)
			t.Fatalf("environment %d: %s %s\n%s\n%s", e.ID, current.State, current.Error, logs, live)
		}
		return current
	}
	read := func(e *Environment, path string) map[string]string {
		t.Helper()
		client := http.Client{Timeout: 5 * time.Second}
		response, err := client.Get(e.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result map[string]string
		if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	one := start(CreateInput{Branch: "main"})
	if read(one, "/set")["value"] != "one" {
		t.Fatal("first preview cannot persist data")
	}
	if err := os.WriteFile(filepath.Join(root, "server.cjs"), []byte(strings.Replace(server, "'one'", "'two'", 1)), 0644); err != nil {
		t.Fatal(err)
	}
	job := func() int {
		t.Helper()
		request, _ := json.Marshal(map[string]string{"repoPath": root, "intent": "implement"})
		result, err := store.DB.Exec(`INSERT INTO agent_job(task,status,requestJson) VALUES(1,'completed',?)`, string(request))
		if err != nil {
			t.Fatal(err)
		}
		id, _ := result.LastInsertId()
		artifact, _ := json.Marshal(map[string]string{"branch": "main", "workspace": root})
		if _, err = store.DB.Exec(`INSERT INTO agent_run(job,artifactJson) VALUES(?,?)`, id, string(artifact)); err != nil {
			t.Fatal(err)
		}
		return int(id)
	}
	two := start(CreateInput{JobID: job()})
	if result := read(two, "/"); result["version"] != "two" || result["value"] != "" {
		t.Fatalf("dirty source or data isolation failed: %#v", result)
	}
	if one.Commit != two.Commit || one.Digest == two.Digest {
		t.Fatal("snapshot identity did not include uncommitted work")
	}
	if read(one, "/")["version"] != "one" {
		t.Fatal("building second version changed first preview")
	}
	// Network isolation is exercised against a known healthy second service,
	// not inferred from the mere presence of a NetworkPolicy resource.
	ip, err := runtime.kubectl(ctx, two, nil, "-n", namespace(token, two), "get", "service", "app", "-o", "jsonpath={.spec.clusterIP}")
	if err != nil {
		t.Fatal(err)
	}
	probe := "fetch('http://" + string(ip) + ":8000',{signal:AbortSignal.timeout(2000)}).then(()=>process.exit(1)).catch(()=>process.exit(0))"
	if output, err := runtime.kubectl(ctx, one, nil, "-n", namespace(token, one), "exec", "deployment/app", "--", "node", "-e", probe); err != nil {
		t.Fatalf("cross-preview traffic was not denied: %s %v", output, err)
	}
	if err = store.Action(one.ID, "stop"); err != nil {
		t.Fatal(err)
	}
	one, _ = store.Get(one.ID)
	s.reconcile(ctx, one)
	one, _ = store.Get(one.ID)
	if one.State != "stopped" {
		t.Fatalf("stop failed: %s", one.Error)
	}
	if err = store.Action(one.ID, "start"); err != nil {
		t.Fatal(err)
	}
	one, _ = store.Get(one.ID)
	s.reconcile(ctx, one)
	one, _ = store.Get(one.ID)
	if one.State != "ready" || read(one, "/")["value"] != "one" {
		t.Fatalf("restart lost data: %s", one.Error)
	}
	runtime.Close()
	s.reconcile(ctx, two)
	two, _ = store.Get(two.ID)
	if two.State != "ready" || read(two, "/")["version"] != "two" {
		t.Fatal("could not recover localhost access after closing port-forwards")
	}
	if err = os.WriteFile(filepath.Join(root, "math.cjs"), []byte("module.exports = (a,b) => a-b;"), 0644); err != nil {
		t.Fatal(err)
	}
	failed, err := s.Create(ctx, 1, CreateInput{JobID: job()})
	if err != nil {
		t.Fatal(err)
	}
	s.reconcile(ctx, failed)
	failed, _ = store.Get(failed.ID)
	if failed.State != "failed" || failed.TestState != "failed" || failed.TestExitCode == nil || *failed.TestExitCode == 0 {
		t.Fatalf("test failure was not preserved: %#v", failed)
	}
	if raw, err := runtime.kubectl(ctx, failed, nil, "-n", namespace(token, failed), "get", "deployment", "app", "--ignore-not-found", "-o", "name"); err != nil || len(raw) > 0 {
		t.Fatalf("failed tests started a preview: %s %v", raw, err)
	}
	if err = store.Action(one.ID, "delete"); err != nil {
		t.Fatal(err)
	}
	one, _ = store.Get(one.ID)
	s.reconcile(ctx, one)
	one, _ = store.Get(one.ID)
	if one.State != "deleted" {
		t.Fatalf("delete failed: %s", one.Error)
	}
	if exists, err := runtime.owned(ctx, two); err != nil || !exists {
		t.Fatal("deleting first preview damaged the second")
	}
	if read(two, "/")["version"] != "two" {
		t.Fatal("second preview stopped unexpectedly")
	}
	logs, _ := store.Logs(failed.ID)
	if !strings.Contains(logs, "addition") {
		t.Fatal("test logs were not retained")
	}
	t.Log("Verified two code versions, uncommitted source, isolated data, denied cross-preview traffic, passing/failing tests, stop/start persistence, access recovery, and scoped deletion.")
}
