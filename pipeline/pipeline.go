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

// Inspect is a shell command to dump gains for a set of frequencies all along an audio file.
func Inspect() func(context.Context) *exec.Cmd {
	const cmd = "ffmpeg -v quiet -i pipe:0 -f f32le -ac 1 -ar 44100 pipe:1 | sox -q -V0 -b 32 -r 44100 -c 1 -e floating-point -t raw - -n stat -freq -rms 2>&1 | grep -E '[^ ]+  [^ ]+'"
	// Convert to floating point audio and pass it to sox.
	return func(ctx context.Context) *exec.Cmd {
		return exec.Command("sh", "-c", cmd)
	}
}
