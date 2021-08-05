package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mkcdj"
	"mkcdj/bpm"
	"mkcdj/ffmpeg"
	"mkcdj/repository"
	"os"
	"strconv"
)

var verbose = flag.Bool("v", false, "Print additional information")

func main() {
	flag.Parse()

	if err := run(flag.Args()...); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[0], err)
		os.Exit(1)
	}
}

func run(args ...string) error {
	if *verbose {
		log.SetOutput(os.Stderr)
	} else {
		log.SetOutput(io.Discard)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	switch {
	case len(args) < 1:
		return usage()
	case args[0] == "analyze" && len(args) == 3:
		return analyze(ctx, args[1], args[2])
	case args[0] == "compile" && len(args) == 2:
		return compile(ctx, args[1])
	case args[0] == "list" && len(args) == 1:
		return list(ctx, os.Stdout)
	case args[0] == "files" && len(args) == 1:
		return files(ctx, os.Stdout)
	case args[0] == "prune" && len(args) == 1:
		return prune(ctx)
	default:
		return usage()
	}
}

func analyze(ctx context.Context, preset, path string) error {
	switch p, err := lookup(preset); {
	case err != nil:
		return err
	default:
		return mkcdj.New(opts()...).Analyze(ctx, path, p)
	}
}

func compile(ctx context.Context, path string) error {
	return mkcdj.New(opts()...).Compile(ctx, path)
}

func list(ctx context.Context, out io.Writer) error {
	return mkcdj.New(repo()).List(out)
}

func files(ctx context.Context, out io.Writer) error {
	return mkcdj.New(repo()).Files(out)
}

func prune(ctx context.Context) error {
	return mkcdj.New(repo()).Prune()
}

const help string = `invalid parameters
usage:
  mkcdj [-v] analyze PRESET AUDIO_FILE
  mkcdj [-v] compile DEST_DIRECTORY
  mkcdj [-v] list
  mkcdj [-v] files
  mkcdj [-v] prune`

func usage() error { return errors.New(help) }

func opts() []mkcdj.Option {
	return []mkcdj.Option{
		repo(),
		mkcdj.WithPipeline("analyze", ffmpeg.F32LE),
		mkcdj.WithPipeline("convert", ffmpeg.AudioOut),
		mkcdj.WithPipeline("waveform", ffmpeg.PNGWaveform),
		mkcdj.WithPipeline("spectrum", ffmpeg.PNGSpectrum),
		mkcdj.WithBPMScanFunc(bpm.Scan),
	}
}

func repo() mkcdj.Option {
	return mkcdj.WithRepository(
		repository.JSONFile(env("MKCDJ_STORE", "/tmp/mkcdj.json")),
	)
}

func lookup(name string) (mkcdj.Preset, error) {
	switch bpm, err := strconv.ParseFloat(name, 64); {
	case err == nil:
		return mkcdj.PresetFromBPM(bpm)
	case name == "default":
		return mkcdj.Default, nil
	default:
		return mkcdj.PresetFromName(name)
	}
}

func env(name, fallback string) string {
	if val, ok := os.LookupEnv(name); ok {
		return val
	}
	return fallback
}
