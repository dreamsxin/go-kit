package main

import (
	"errors"
	"os/exec"
	"testing"
)

// TestCrashExitCodesAreToldFromVerdicts covers the distinction the retry rests on.
// A compiler that ran and found type errors must fail on the first attempt; a
// compiler that died must not be reported as if it had an opinion about the code.
func TestCrashExitCodesAreToldFromVerdicts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		code int
		want bool
	}{
		{"type errors", 1, false},
		{"bad configuration", 2, false},
		{"an ordinary large status", 0x7FFFFFFF, false},
		{"killed by a signal", -1, true},
		{"windows access violation", 0xC0000005, true},
		{"windows stack buffer overrun", 0xC0000409, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isCrashExitCode(tc.code); got != tc.want {
				t.Fatalf("isCrashExitCode(%#x) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}

// TestSetupFailuresAreNotCrashes proves a failure that never became a process is
// not retried. Retrying a missing binary or an unexecutable path only doubles the
// wait before the same message.
func TestSetupFailuresAreNotCrashes(t *testing.T) {
	t.Parallel()
	if crashedProcess(errors.New("exec: \"npx\": executable file not found in %PATH%")) {
		t.Error("a lookup failure was classified as a crash")
	}
	if crashedProcess(&exec.Error{Name: "npx", Err: exec.ErrNotFound}) {
		t.Error("an exec.Error was classified as a crash")
	}
}
