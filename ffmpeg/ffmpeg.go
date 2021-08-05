// Package ffmpeg contains the shell commands used to perform audio transcoding.
package ffmpeg

import (
	"context"
	"os/exec"
)

var (
	a = [...]string{"ffmpeg", "-v", "quiet", "-i", "pipe:0", "-f", "f32le", "-ac", "1", "-ar", "44100", "pipe:1"}
	b = [...]string{"ffmpeg", "-v", "quiet", "-i", "pipe:0", "-f", "wav", "-ac", "2", "-ar", "44100", "-acodec", "pcm_s24le", "pipe:1"}
	c = [...]string{"ffmpeg", "-v", "quiet", "-i", "pipe:0", "-lavfi", "showwavespic=s=4096x2048:colors=#5294E2", "-f", "image2", "pipe:1"}
	d = [...]string{"ffmpeg", "-v", "quiet", "-i", "pipe:0", "-lavfi", "showspectrumpic=s=4096x2048:color=cool:start=0:stop=24000", "-f", "image2", "pipe:1"}
)

func F32LE(ctx context.Context) *exec.Cmd       { return exec.CommandContext(ctx, a[0], a[1:]...) } //nolint:gosec
func AudioOut(ctx context.Context) *exec.Cmd    { return exec.CommandContext(ctx, b[0], b[1:]...) } //nolint:gosec
func PNGWaveform(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, c[0], c[1:]...) } //nolint:gosec
func PNGSpectrum(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, d[0], d[1:]...) } //nolint:gosec
