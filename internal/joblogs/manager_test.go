package joblogs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestManagerPersistsHistoryAndResumesSequence(t *testing.T) {
	root := t.TempDir()
	m, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := m.Append(context.Background(), "job-1", "steamcmd", Output, "line one")
	if err != nil || first.Seq != 1 {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := m.Append(context.Background(), "job-1", "task", Info, "line two")
	if err != nil || second.Seq != 2 {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	page, err := reopened.Read(context.Background(), "job-1", Query{Limit: 50})
	if err != nil || len(page.Records) != 2 || page.NextSeq != 2 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	next, err := reopened.Append(context.Background(), "job-1", "task", Warn, "line three")
	if err != nil || next.Seq != 3 {
		t.Fatalf("next=%+v err=%v", next, err)
	}
}

func TestManagerSerializesConcurrentAppends(t *testing.T) {
	m, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, appendErr := m.Append(context.Background(), "job-1", "task", Info, "line"); appendErr != nil {
				t.Errorf("append: %v", appendErr)
			}
		}()
	}
	wg.Wait()
	page, err := m.Read(context.Background(), "job-1", Query{Limit: 100})
	if err != nil || len(page.Records) != 40 {
		t.Fatalf("records=%d err=%v", len(page.Records), err)
	}
	for index, record := range page.Records {
		if record.Seq != uint64(index+1) {
			t.Fatalf("record[%d].Seq=%d", index, record.Seq)
		}
	}
}

func TestManagerSubscriptionReceivesNewRecords(t *testing.T) {
	m, err := Open(t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	_, _ = m.Append(context.Background(), "job-1", "task", Info, "history")
	replay, stream, cancel, err := m.Subscribe(context.Background(), "job-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if len(replay) != 1 || replay[0].Message != "history" {
		t.Fatalf("replay=%+v", replay)
	}
	_, _ = m.Append(context.Background(), "job-1", "steamcmd", Output, "live")
	select {
	case record := <-stream:
		if record.Seq != 2 || record.Message != "live" {
			t.Fatalf("record=%+v", record)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live record")
	}
}

func TestManagerFinalizesToConfiguredLimitAndMarksTruncation(t *testing.T) {
	root := t.TempDir()
	m, err := Open(root, Options{TerminalLimit: 1024})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		_, _ = m.Append(context.Background(), "job-1", "steamcmd", Output, strings.Repeat("x", 80))
	}
	before, err := os.Stat(filepath.Join(root, "job-1.jsonl"))
	if err != nil || before.Size() <= 1024 {
		t.Fatalf("before=%v err=%v", before, err)
	}
	if err := m.Finalize(context.Background(), "job-1"); err != nil {
		t.Fatal(err)
	}
	page, err := m.Read(context.Background(), "job-1", Query{Limit: 100})
	if err != nil || !page.Truncated || len(page.Records) == 0 {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if page.Records[0].Source != "task" || page.Records[0].Level != Warn || !strings.Contains(page.Records[0].Message, "truncated") {
		t.Fatalf("marker=%+v", page.Records[0])
	}
}

func TestManagerDeleteAndCleanupOrphans(t *testing.T) {
	root := t.TempDir()
	m, err := Open(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = m.Append(context.Background(), "keep", "task", Info, "keep")
	_, _ = m.Append(context.Background(), "drop", "task", Info, "drop")
	if err := m.CleanupOrphans(context.Background(), map[string]bool{"keep": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "drop.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("drop stat err=%v", err)
	}
	if err := m.Delete(context.Background(), "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "keep.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("keep stat err=%v", err)
	}
}
