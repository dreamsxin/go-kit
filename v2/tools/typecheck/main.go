package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dreamsxin/go-kit-tools/v2/internal/releaseconfig"
)

// typeCheckAttempts is how many times a configuration is checked when the
// compiler process dies rather than reporting on the code.
const typeCheckAttempts = 2

func main() {
	version := flag.String("typescript-version", releaseconfig.TypeScriptCompilerVersion, "TypeScript compiler version")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./tools/typecheck [flags] <tsconfig> [<tsconfig> ...]")
		os.Exit(2)
	}

	npx, err := exec.LookPath("npx")
	if err != nil {
		fmt.Fprintln(os.Stderr, "typecheck requires Node.js and npx on PATH")
		os.Exit(2)
	}
	for _, config := range flag.Args() {
		absConfig, err := filepath.Abs(config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve tsconfig %q: %v\n", config, err)
			os.Exit(2)
		}
		if info, err := os.Stat(absConfig); err != nil || info.IsDir() {
			fmt.Fprintf(os.Stderr, "tsconfig not found: %s\n", absConfig)
			os.Exit(2)
		}

		fmt.Printf("type-checking %s with TypeScript %s\n", absConfig, *version)
		if err := typeCheck(npx, *version, absConfig); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

// typeCheck runs the compiler over one configuration, and runs it again when the
// compiler itself died.
//
// A crashed compiler is not a verdict on the code. It prints none of the
// diagnostics a type error prints, and reporting it as a failed type-check sends
// whoever reads the log looking for a mistake in the checked SDK that is not
// there — which is what happened when tsc 7.0.2 exited 0xC0000409, the stack
// check its runtime performs on detecting corruption. A type error, by contrast,
// is deterministic: it is reported on the first attempt and never retried.
func typeCheck(npx, version, config string) error {
	for attempt := 1; ; attempt++ {
		cmd := exec.Command(npx,
			"--yes",
			"--package", "typescript@"+version,
			"tsc",
			"-p", config,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()

		err := cmd.Run()
		if err == nil {
			return nil
		}
		if !crashedProcess(err) {
			return fmt.Errorf("TypeScript type-check failed for %s: %w", config, err)
		}
		if attempt >= typeCheckAttempts {
			return fmt.Errorf("the TypeScript compiler crashed for %s on %d attempts (%v): this is the "+
				"compiler dying rather than a type error in the checked code", config, attempt, err)
		}
		fmt.Fprintf(os.Stderr, "the TypeScript compiler crashed for %s (%v); retrying once\n", config, err)
	}
}

// crashedProcess reports whether a command's failure describes a process that
// died rather than a program that ran and disagreed.
func crashedProcess(err error) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		// Not a completed process at all: a missing binary, or a path that could
		// not be executed. That is a setup failure, not a crash to retry.
		return false
	}
	return isCrashExitCode(exit.ExitCode())
}

// isCrashExitCode reports whether an exit code describes a process that was
// killed or that failed inside its own runtime.
//
// A negative code is a signal on Unix, where os/exec reports -1. A code at or
// above 0xC0000000 is a Windows NTSTATUS failure such as 0xC0000005 (access
// violation) or 0xC0000409 (stack buffer overrun). A compiler that ran to
// completion reports 1 for type errors and 2 for a bad configuration, so those
// stay failures.
func isCrashExitCode(code int) bool {
	return code < 0 || uint32(code) >= 0xC0000000
}
