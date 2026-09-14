package main

import "testing"

func TestDispatchRejectsNoArgumentsAndUnknownCommands(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"an unknown command", []string{"deploy"}},
		{"a flag where a command belongs", []string{"--help"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if code := dispatch(tc.args); code != exitFailure {
				t.Fatalf("want exit %d, got %d", exitFailure, code)
			}
		})
	}
}

func TestHelpListsEveryVisibleCommandAndHidesServe(t *testing.T) {
	// The table is help's source, so a command can never exist without being listed —
	// but `serve` must stay out: it is an implementation detail of start (R3), not
	// something to reach for.
	visible := 0
	for _, c := range commands {
		if c.name == "serve" && !c.hidden {
			t.Error("serve must be hidden from help")
		}
		if !c.hidden {
			visible++
		}
		if c.run == nil {
			t.Errorf("command %q has no implementation", c.name)
		}
		if c.summary == "" {
			t.Errorf("command %q has no summary, so help would print a blank line", c.name)
		}
	}
	if visible != 7 {
		t.Fatalf("want 7 visible commands, got %d", visible)
	}
}

// The separator is what lets a suite take flags of its own without wrap trying to
// parse them, so both forms must reach the same place.
func TestWrapStripsTheSeparator(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"with the documented separator", []string{"--", "npm", "test"}, []string{"npm", "test"}},
		{"without it", []string{"npm", "test"}, []string{"npm", "test"}},
		{"only the separator", []string{"--"}, nil},
		{"a suite with its own flags", []string{"--", "go", "test", "-race", "./..."}, []string{"go", "test", "-race", "./..."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := stripSeparator(tc.args)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestForceFlagParsing(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"--force"}, true},
		{[]string{"-f"}, true},
		{[]string{"--quiet"}, false},
		{[]string{"--quiet", "--force"}, true},
	} {
		if got := hasForce(tc.args); got != tc.want {
			t.Errorf("hasForce(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}
