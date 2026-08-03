package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/acourtiol/magelift/internal/config"
)

func main() {
	check := flag.Bool("check", false, "fail if generated files are stale")
	root := flag.String("root", ".", "repository root containing schema and docs")
	flag.Parse()

	outputs := []struct {
		path string
		data []byte
	}{
		{path: "schema/magelift.schema.json", data: configOutput(config.SchemaJSON())},
		{path: "docs/configuration.md", data: config.ReferenceMarkdown()},
	}
	for _, output := range outputs {
		path, expected := filepath.Join(*root, output.path), output.data
		if *check {
			actual, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(actual, expected) {
				if err == nil {
					err = errors.New("content differs")
				}
				fatalf("%s is stale: %v; run go generate ./internal/config", path, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fatalf("create output directory: %v", err)
		}
		if err := os.WriteFile(path, expected, 0o644); err != nil {
			fatalf("write %s: %v", path, err)
		}
	}
}

func configOutput(data []byte, err error) []byte {
	if err != nil {
		fatalf("generate schema: %v", err)
	}
	return data
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
