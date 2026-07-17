package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dreamsxin/go-kit/v2/cmd/microgen/generator"
)

func main() {
	version := flag.String("typescript-version", generator.TypeScriptCompilerVersion, "TypeScript compiler version")
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
		cmd := exec.Command(npx,
			"--yes",
			"--package", "typescript@"+*version,
			"tsc",
			"-p", absConfig,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = os.Environ()
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "TypeScript type-check failed for %s: %v\n", absConfig, err)
			os.Exit(1)
		}
	}
}
