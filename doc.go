// Command trainsty is a local end-to-end test scheduler: a single-binary Unix
// daemon that serializes E2E test runs across multiple repository clones on one
// developer machine.
//
// Four clones of a repository on one laptop cannot run two E2E suites at once —
// they contend for fixed ports, local database locks, and enough memory that two
// headless browser fleets thrash the machine. trainsty is a semaphore of depth
// one that queues them instead, without taking the developer's terminal away.
//
// The daemon is a traffic light: it owns the lock and the queue and nothing else.
// It never runs, supervises or captures a test suite. See
// knowledge/index.md for the full design, and .specify/memory/constitution.md for
// the five principles that constrain it.
package main
