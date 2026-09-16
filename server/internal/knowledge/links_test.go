package knowledge

import (
	"errors"
	"github.com/waseem-polus/aycorn/server/internal/taskownership"
	"sync"
	"testing"
)

func TestGitHubReferenceURLs(t *testing.T) {
	for _, test := range []struct{ input, url, kind, reference string }{
		{"http://www.github.com/Owner/Repo/pull/0012?diff=split#review", "https://github.com/owner/repo/pull/12", "pull_request", "owner/repo #12"},
		{"https://github.com/Owner/Repo/tree/Feature/new-ui", "https://github.com/owner/repo/tree/Feature/new-ui", "branch", "owner/repo · Feature/new-ui"},
		{"https://github.com/Owner/Repo/tree/feature%2Fnew-ui", "https://github.com/owner/repo/tree/feature/new-ui", "branch", "owner/repo · feature/new-ui"},
	} {
		url, kind, ref, err := NormalizeGitHubURL(test.input)
		if err != nil || url != test.url || kind != test.kind || ref != test.reference {
			t.Fatalf("%q: %q %q %q %v", test.input, url, kind, ref, err)
		}
	}
	for _, input := range []string{"javascript:alert(1)", "https://evil.example/owner/repo/pull/1", "https://github.com.evil.example/a/b/tree/main", "https://secret@github.com/a/b/pull/1", "https://github.com:443/a/b/pull/1", "https://github.com/a/b/pull/0", "https://github.com/a/b/pull/1/files", "https://github.com/a/b/tree/", "https://github.com/a/b/tree/../secret", "https://github.com/a/b/tree/x%0Ay", "https://github.com/a/b/issues/3"} {
		if _, _, _, err := NormalizeGitHubURL(input); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestTaskLinksIdempotencyRevisionsAndOwnership(t *testing.T) {
	s := testStore(t)
	if _, err := s.DB.Exec(`INSERT INTO stage(id,workflow,name,type,color,icon,position) VALUES(101,101,'Open','open','gray','circle',1);
INSERT INTO checklist(id,project,name) VALUES(101,101,'Tasks');
INSERT INTO task(id,checklist,stage,type,name,priority,body) VALUES(101,101,101,1,'Task','Medium','');`); err != nil {
		t.Fatal(err)
	}
	input := LinkInput{URL: "https://github.com/Owner/Repo/pull/1", Label: "Feature"}
	var wg sync.WaitGroup
	ids := make(chan int, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			link, err := s.PutLink(101, 0, input, nil)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- link.ID
		}()
	}
	wg.Wait()
	close(ids)
	var id int
	for got := range ids {
		if id != 0 && id != got {
			t.Fatal("duplicate created")
		}
		id = got
	}
	links, err := s.Links(101)
	if err != nil || len(links) != 1 {
		t.Fatalf("%+v %v", links, err)
	}
	input.Label = "Renamed"
	input.Revision = 1
	link, err := s.PutLink(101, id, input, nil)
	if err != nil || link.Label != "Renamed" || link.Revision != 2 {
		t.Fatalf("%+v %v", link, err)
	}
	if _, err := s.PutLink(101, id, input, nil); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO agent_job(id,task,status) VALUES(41,101,'running')"); err != nil {
		t.Fatal(err)
	}
	external, own := 0, 41
	if _, err := s.PutLink(101, 0, LinkInput{URL: "https://github.com/owner/repo/tree/new"}, &external); !errors.Is(err, taskownership.ErrBusy) {
		t.Fatal(err)
	}
	if err := s.DeleteLink(101, id, 2, &external); !errors.Is(err, taskownership.ErrBusy) {
		t.Fatal(err)
	}
	if _, err := s.PutLink(101, 0, LinkInput{URL: "https://github.com/owner/repo/tree/new"}, &own); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("UPDATE agent_job SET status='completed' WHERE id=41"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLink(101, id, 2, &own); !errors.Is(err, taskownership.ErrNotOwner) {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("INSERT INTO conductor_task(task,project,state,expectedStage) VALUES(101,101,'waiting',101)"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLink(101, id, 2, &external); !errors.Is(err, taskownership.ErrBusy) {
		t.Fatal(err)
	}
	// Human edits remain possible while AI is working.
	if err := s.DeleteLink(101, id, 2, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("DELETE FROM task WHERE id=101"); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := s.DB.QueryRow("SELECT count(*) FROM task_github_link").Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("orphan links: %d %v", remaining, err)
	}
}
