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
		return errUsage
	case args[0] == "analyze" && len(args) == 3:
		return analyze(ctx, args[1], args[2])
	case args[0] == "compile" && len(args) == 2:
		return compile(ctx, args[1])
	case args[0] == "refresh" && len(args) == 1:
		return refresh(ctx)
	case args[0] == "list" && len(args) == 1:
		return list(os.Stdout)
	case args[0] == "files" && len(args) == 1:
		return files(os.Stdout)
	case args[0] == "prune" && len(args) == 1:
		return prune()
	default:
		return errUsage
	}
}

func analyze(ctx context.Context, preset, path string) error {
	switch p, err := lookup(preset); {
	case err != nil:
		return err
	default:
		return mkcdj.New(opts[:]...).Analyze(ctx, path, p)
	}
}

func compile(ctx context.Context, path string) error {
	return mkcdj.New(opts[:]...).Compile(ctx, path)
}

func refresh(ctx context.Context) error {
	return mkcdj.New(opts[:]...).Refresh(ctx)
}

func list(out io.Writer) error {
	return mkcdj.New(repo).List(out)
}

func files(out io.Writer) error {
	return mkcdj.New(repo).Files(out)
}

func prune() error {
	return mkcdj.New(repo).Prune()
}

const help string = `invalid parameters
usage:
  mkcdj [-v] analyze PRESET AUDIO_FILE
  mkcdj [-v] compile DEST_DIRECTORY
  mkcdj [-v] refresh
  mkcdj [-v] list
  mkcdj [-v] files
  mkcdj [-v] prune`

var errUsage = errors.New(help)

var repo = mkcdj.WithRepository(
	repository.JSONFile(env("MKCDJ_STORE", "/tmp/mkcdj.json")),
)

var opts = [...]mkcdj.Option{
	repo,
	mkcdj.WithPipeline(mkcdj.Analyze, mkcdj.PipelineFunc(ffmpeg.F32LE)),
	mkcdj.WithPipeline(mkcdj.Convert, mkcdj.PipelineFunc(ffmpeg.AudioOut)),
	mkcdj.WithPipeline(mkcdj.Waveform, mkcdj.PipelineFunc(ffmpeg.PNGWaveform)),
	mkcdj.WithPipeline(mkcdj.Spectrum, mkcdj.PipelineFunc(ffmpeg.PNGSpectrum)),
	mkcdj.WithBPMScanFunc(bpm.Scan),
}

func lookup(name string) (mkcdj.Preset, error) {
	switch bpm, err := strconv.ParseFloat(name, 64); {
	case err == nil:
		return mkcdj.PresetFromBPM(bpm)
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
