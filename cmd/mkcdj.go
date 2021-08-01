package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mkcdj"
	"mkcdj/pipeline"
	"mkcdj/repository"
	"os"
)

func main() {
	if err := run(os.Args...); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[0], err)
		os.Exit(1)
	}
}

func run(args ...string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	switch {
	case len(args) < 2:
		return usage()
	case args[1] == "analyze" && len(args) == 4:
		return analyze(ctx, args[2], args[3])
	case args[1] == "compile" && len(args) == 3:
		return compile(ctx, args[2])
	case args[1] == "list" && len(args) == 2:
		return list(ctx, os.Stdout)
	case args[1] == "prune" && len(args) == 2:
		return prune(ctx)
	default:
		return usage()
	}
}

func analyze(ctx context.Context, preset, path string) error {
	p, err := mkcdj.PresetFromName(preset)
	if preset != "default" && err != nil {
		return err
	}

	min, max := p.Range()

	a, i := pipeline.Analyze(min, max), pipeline.Inspect()

	m := mkcdj.New(repo(), mkcdj.WithAnalyzeFunc(a), mkcdj.WithInspectFunc(i))

	return m.Analyze(ctx, path)
}

func compile(ctx context.Context, dest string) error {
	return mkcdj.New(repo(), mkcdj.WithConvertFunc(pipeline.Convert())).Compile(ctx, dest)
}

func list(ctx context.Context, out io.Writer) error {
	return mkcdj.New(repo()).List(out)
}

func prune(ctx context.Context) error {
	return mkcdj.New(repo()).Prune(ctx)
}

const help string = `invalid parameters

Usage:
  mkcdj analyze PRESET AUDIO_FILE
  mkcdj compile DEST_DIRECTORY
  mkcdj list
  mkcdj prune`

func usage() error { return errors.New(help) }

func repo() mkcdj.Option {
	return mkcdj.WithRepository(
		repository.JSONFile(env("MKCDJ_STORE", "/tmp/mkcdj.json")),
	)
}

func env(name, fallback string) string {
	if val, ok := os.LookupEnv(name); ok {
		return val
	}
	return fallback
}
