package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: makeguard <command> [args...]")
	}

	switch os.Args[1] {
	case "dir-exists":
		if len(os.Args) != 4 {
			fail("usage: makeguard dir-exists <dir> <message>")
		}
		dir := os.Args[2]
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			fail(os.Args[3])
		}
	case "remove":
		if len(os.Args) < 3 {
			fail("usage: makeguard remove <file> [file...]")
		}
		for _, path := range os.Args[2:] {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				fail("remove %s: %v", path, err)
			}
		}
	case "proto":
		if len(os.Args) != 3 {
			fail("usage: makeguard proto <proto-dir>")
		}
		runProto(os.Args[2])
	default:
		fail("unknown command: %s", os.Args[1])
	}
}

func runProto(root string) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		fail("proto directory %q not found", root)
	}

	byDir := map[string][]string{}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".proto" {
			return nil
		}
		dir := filepath.Dir(path)
		byDir[dir] = append(byDir[dir], filepath.Base(path))
		return nil
	})
	if err != nil {
		fail("scan proto dir %q: %v", root, err)
	}
	if len(byDir) == 0 {
		fail("no .proto files found under %q", root)
	}

	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	fmt.Println(">>> Generating protobuf Go files...")
	for _, dir := range dirs {
		files := byDir[dir]
		sort.Strings(files)
		fmt.Printf("  protoc: %s\n", dir)
		args := []string{
			"--proto_path=" + dir,
			"--go_out=" + dir,
			"--go_opt=paths=source_relative",
			"--go-grpc_out=" + dir,
			"--go-grpc_opt=paths=source_relative",
		}
		args = append(args, files...)
		cmd := exec.Command("protoc", args...)
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fail("protoc failed in %s: %v", dir, err)
		}
	}
	fmt.Println(">>> Done. pb.go files generated.")
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
