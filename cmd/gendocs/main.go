package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/magelift/magelift/internal/cli"
	"github.com/spf13/cobra"
)

const outputPath = "docs/cli-reference.md"

func main() {
	check := len(os.Args) == 2 && os.Args[1] == "--check"
	if len(os.Args) > 1 && !check {
		fmt.Fprintln(os.Stderr, "usage: gendocs [--check]")
		os.Exit(2)
	}
	root := cli.New()
	root.InitDefaultHelpCmd()
	root.InitDefaultHelpFlag()
	contents := render(root)
	if check {
		current, err := os.ReadFile(outputPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				fmt.Fprintln(os.Stderr, "CLI reference is missing; run go run ./cmd/gendocs")
			} else {
				fmt.Fprintf(os.Stderr, "read %s: %v\n", outputPath, err)
			}
			os.Exit(1)
		}
		if !bytes.Equal(current, contents) {
			fmt.Fprintln(os.Stderr, "CLI reference is out of date; run go run ./cmd/gendocs")
			os.Exit(1)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(outputPath, contents, 0o644); err != nil {
		panic(err)
	}
}

func render(root *cobra.Command) []byte {
	var output strings.Builder
	output.WriteString("# CLI reference\n\n")
	output.WriteString("This page is generated from the Cobra command tree. Do not edit it by hand.\n\n")
	output.WriteString("## Global options\n\n```text\n")
	output.WriteString(root.PersistentFlags().FlagUsages())
	output.WriteString("```\n")
	renderCommand(&output, root, 0)
	return []byte(output.String())
}

func renderCommand(output *strings.Builder, command *cobra.Command, depth int) {
	children := availableCommands(command)
	if command != rootCommand(command) {
		heading := depth + 1
		output.WriteString(strings.Repeat("#", heading) + " " + command.CommandPath() + "\n\n")
		if command.Short != "" {
			output.WriteString(command.Short + "\n\n")
		}
		output.WriteString("```text\n" + command.UseLine() + "\n```\n")
		if flags := command.LocalNonPersistentFlags().FlagUsages(); flags != "" {
			output.WriteString("\nOptions:\n\n```text\n" + flags + "```\n")
		}
	}
	for _, child := range children {
		renderCommand(output, child, depth+1)
	}
}

func availableCommands(command *cobra.Command) []*cobra.Command {
	children := make([]*cobra.Command, 0, len(command.Commands()))
	for _, child := range command.Commands() {
		if child.IsAvailableCommand() && !child.Hidden {
			children = append(children, child)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].CommandPath() < children[j].CommandPath() })
	return children
}

func rootCommand(command *cobra.Command) *cobra.Command {
	for command.HasParent() {
		command = command.Parent()
	}
	return command
}
