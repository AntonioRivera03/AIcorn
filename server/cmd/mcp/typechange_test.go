package main

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

// readTaskState is a small snapshot of the state a type change must not
// disturb: the task's body and its agent_job rows (a chat-type task's
// message history).
type readTaskState struct {
	body string
	jobs []string
}

func readTask1State(t *testing.T, db *sql.DB) readTaskState {
	t.Helper()
	var state readTaskState
	if err := db.QueryRow("SELECT body FROM task WHERE id=1").Scan(&state.body); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query("SELECT requestJson FROM agent_job WHERE task=1 ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var requestJson string
		if err := rows.Scan(&requestJson); err != nil {
			t.Fatal(err)
		}
		state.jobs = append(state.jobs, requestJson)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return state
}

// TestUpdateTaskTypeChangePreservesBodyAndChatHistory covers switching a
// task's type between a document-viewMode type and the built-in chat type:
// update_task's UPDATE only ever touches the task's type column, so its body
// (the Plate document, hidden but not cleared while a task reads as chat —
// see AIService.StartChat) and its agent_job rows (the chat's message
// history) must survive the round trip in both directions.
func TestUpdateTaskTypeChangePreservesBodyAndChatHistory(t *testing.T) {
	taskService, docType, chatType := newNotFoundTestDB(t)
	db := taskService.TaskRepo.DB

	body := `[{"type":"p","children":[{"text":"Original body"}]}]`
	if _, err := db.Exec("UPDATE task SET body=? WHERE id=1", body); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO agent_job(task,status,requestJson) VALUES
(1,'completed','{"chat":{"clientKey":"k1"},"message":"first"}'),
(1,'completed','{"chat":{"clientKey":"k2"},"message":"second"}')`); err != nil {
		t.Fatal(err)
	}

	want := readTask1State(t, db)
	if want.body != body || len(want.jobs) != 2 {
		t.Fatalf("seed mismatch: %+v", want)
	}

	ts := &toolset{taskService: taskService}
	ctx := context.Background()

	if _, out, err := ts.updateTask(ctx, nil, UpdateTaskInput{TaskID: 1, TypeID: &chatType}); err != nil || !out.Ok {
		t.Fatalf("document -> chat: err=%v out=%+v", err, out)
	}
	if got := readTask1State(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("document -> chat changed task state: got %+v; want %+v", got, want)
	}

	if _, out, err := ts.updateTask(ctx, nil, UpdateTaskInput{TaskID: 1, TypeID: &docType}); err != nil || !out.Ok {
		t.Fatalf("chat -> document: err=%v out=%+v", err, out)
	}
	if got := readTask1State(t, db); !reflect.DeepEqual(got, want) {
		t.Fatalf("chat -> document changed task state: got %+v; want %+v", got, want)
	}

	var currentType int
	if err := db.QueryRow("SELECT type FROM task WHERE id=1").Scan(&currentType); err != nil {
		t.Fatal(err)
	}
	if currentType != docType {
		t.Fatalf("task type = %d; want %d (document)", currentType, docType)
	}
}
