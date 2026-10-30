package logic

import (
	"errors"
	"testing"
	"time"
)

func TestChangelogWritesStopDrains(t *testing.T) {
	apl := &Applier{}
	entered := make(chan struct{})
	release := make(chan struct{})
	written := make(chan error, 1)
	go func() {
		written <- apl.withChangelogWrite(func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	stopping := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		close(stopping)
		apl.StopChangelogWrites()
		close(stopped)
	}()
	<-stopping
	select {
	case <-stopped:
		t.Fatal("cleanup did not wait for in-flight SQL")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup did not finish after SQL completed")
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	// With no database/context, any attempted SQL would panic.
	for _, hint := range []string{"heartbeat", "state", "throttle", "status"} {
		if got, err := apl.WriteChangelog(hint, "late write"); err != nil || got != hint {
			t.Fatalf("late %s: hint=%s err=%v", hint, got, err)
		}
	}
	apl.StopChangelogWrites() // The barrier is idempotent.
}

func TestChangelogWritesErrorsPropagate(t *testing.T) {
	apl := &Applier{}
	want := errors.New("SQL failure")
	if got := apl.withChangelogWrite(func() error { return want }); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	apl.StopChangelogWrites()
	if err := apl.withChangelogWrite(func() error {
		t.Fatal("write executed after cleanup barrier")
		return want
	}); err != nil {
		t.Fatal(err)
	}
}
