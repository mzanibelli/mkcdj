package main

import (
	"context"
	"errors"
	"fmt"
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
	default:
		return usage()
	}
}

func analyze(ctx context.Context, preset, path string) error {
	p, err := mkcdj.PresetFromName(preset)
	if err != nil {
		return err
	}

	min, max := p.Range()
	f := pipeline.Analyze(min, max)

	return mkcdj.New(repo(), mkcdj.WithAnalyzeFunc(f)).Analyze(ctx, path)
}

func compile(ctx context.Context, dest string) error {
	return mkcdj.New(repo(), mkcdj.WithConvertFunc(pipeline.Convert())).Compile(ctx, dest)
}

const help string = `invalid parameters

Usage:
  mkcdj analyze PRESET AUDIO_FILE
  mkcdj compile DEST_DIRECTORY`

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
