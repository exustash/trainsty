package httpapi

import (
	"errors"
	"net"
	"testing"

	"github.com/exustash/trainsty/scheduler"
)

// ADR-011's distinction, which exists because the two remedies differ: one is
// `trainsty stop`, the other is finding out what else wants 45678. Collapsing them
// into "port in use" would send the developer looking in the wrong place.
//
// This test binds the real machine-wide port, so it skips rather than fails when
// something already holds it — the same rule the acceptance suite follows.
func TestListenDistinguishesAnotherProgramFromAnotherDaemon(t *testing.T) {
	if Reachable() {
		t.Skip("a trainsty daemon is already running — stop it to exercise the bind classification")
	}

	// 1. Something that is NOT trainsty holds the port: a plain listener that never
	//    answers /status.
	blocker, err := net.Listen("tcp", Addr)
	if err != nil {
		t.Skipf("could not bind %s to set up the test: %v", Addr, err)
	}

	_, err = Listen()

	if !errors.Is(err, ErrPortTaken) {
		blocker.Close()
		t.Fatalf("want ErrPortTaken when a non-trainsty program holds the port, got %v", err)
	}
	blocker.Close()

	// 2. A real trainsty daemon holds it: the same EADDRINUSE, a different answer,
	//    because /status replies.
	ln, err := Listen()
	if err != nil {
		t.Fatalf("expected a clean bind once the port was free: %v", err)
	}
	srv := New(scheduler.New(), DiscardLogger())
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = ln.Close() })

	if !Reachable() {
		t.Fatal("the daemon under test is not answering /status")
	}
	_, err = Listen()

	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("want ErrAlreadyRunning when another daemon holds the port, got %v", err)
	}
}

func TestReachableIsFalseWithNothingListening(t *testing.T) {
	if Reachable() {
		t.Skip("something is listening on the port")
	}
	if Reachable() {
		t.Fatal("want false when nothing answers")
	}
}
