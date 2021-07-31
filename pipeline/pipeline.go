package pipeline

import (
	"context"
	"fmt"
	"os/exec"
)

// Analyze is a shell command to perform BPM analysis for a given BPM range.
func Analyze(min, max string) func(context.Context) *exec.Cmd {
	// Convert the standard input to mono raw PCM data (32 bits, float, little
	// endian) and pass it to bpm-tools(1) with the given BPM range.
	const tpl = `ffmpeg -v quiet -i pipe:0 -f f32le -ac 1 -ar 44100 pipe:1 | bpm -m %s -x %s -f %%0.0f`

	cmd := fmt.Sprintf(tpl, min, max)

	return func(ctx context.Context) *exec.Cmd {
		//nolint:gosec
		return exec.CommandContext(ctx, "sh", "-c", cmd)
	}
}

// Convert is a shell command to convert audio files to a common format.
func Convert() func(context.Context) *exec.Cmd {
	// Convert the standard intput to a Pioneer-compatible 16 bits stereo WAV at 44110Hz.
	return func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "ffmpeg", "-v", "quiet", "-i", "pipe:0",
			"-f", "wav", "-ac", "2", "-ar", "44100", "-acodec", "pcm_s16le", "pipe:1")
	}
}
