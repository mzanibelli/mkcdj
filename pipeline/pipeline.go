// Package pipeline contains the shell commands used to perform audio analysis.
package pipeline

import (
	"bytes"
	"context"
	"os/exec"
	"text/template"

	_ "embed"
)

// Disclaimer: this file hides all the bad things delegated to obscure shell commands.

var required = []string{"sh", "ffmpeg", "bpm"}

// Check returns an error if a dependency is missing.
func Check() error {
	for _, path := range required {
		if _, err := exec.LookPath(path); err != nil {
			return err
		}
	}
	return nil
}

//go:embed templates/analyze.tpl
var analyze string

// Analyze is a shell command to perform BPM analysis for a given BPM range.
func Analyze(min, max string) func(context.Context) *exec.Cmd {
	data, out := struct{ Min, Max string }{min, max}, bytes.NewBuffer(nil)
	if err := template.Must(template.New("").Parse(analyze)).Execute(out, data); err != nil {
		panic(err)
	}

	return func(ctx context.Context) *exec.Cmd {
		//nolint:gosec
		return exec.CommandContext(ctx, "sh", "-c", out.String())
	}
}

//go:embed templates/convert.tpl
var convert string

// Convert is a shell command to convert audio files to a common format.
func Convert() func(context.Context) *exec.Cmd {
	return func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", convert)
	}
}

//go:embed templates/waveform.tpl
var waveform string

// Waveform is a shell command to compute a PNG with the track waveform.
func Waveform() func(context.Context) *exec.Cmd {
	return func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", waveform)
	}
}

//go:embed templates/spectrogram.tpl
var spectrogram string

// Spectrogram is a shell command to compute a PNG with the track spectrogram.
func Spectrogram() func(context.Context) *exec.Cmd {
	return func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", spectrogram)
	}
}
