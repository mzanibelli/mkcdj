// Package ffmpeg contains the shell commands used to perform audio transcoding.
package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

var (
	a = [...]string{"-v", "quiet", "-y", "-f", "f32le", "-ac", "1", "-ar", "44100"}
	b = [...]string{"-v", "quiet", "-y", "-f", "wav", "-map_metadata", "-1", "-bitexact", "-ac", "2", "-ar", "44100", "-acodec", "pcm_s24le"}
	c = [...]string{"-v", "quiet", "-y", "-lavfi", "showwavespic=s=4096x2048:colors=#5294E2", "-f", "image2"}
	d = [...]string{"-v", "quiet", "-y", "-lavfi", "showspectrumpic=s=4096x2048:color=cool:start=0:stop=24000", "-f", "image2"}
)

func F32LE(ctx context.Context, in io.Reader, out, err io.Writer) error {
	return command(ctx, in, out, err, a[:]...).Run()
}

func AudioOut(ctx context.Context, in io.Reader, out, err io.Writer) error {
	return command(ctx, in, out, err, b[:]...).Run()
}

func PNGWaveform(ctx context.Context, in io.Reader, out, err io.Writer) error {
	return command(ctx, in, out, err, c[:]...).Run()
}

func PNGSpectrum(ctx context.Context, in io.Reader, out, err io.Writer) error {
	return command(ctx, in, out, err, d[:]...).Run()
}

func command(ctx context.Context, in io.Reader, out, err io.Writer, args ...string) *exec.Cmd {
	arg0, ok0 := pipe(in, 0)
	arg1, ok1 := pipe(out, 1)

	args = append([]string{"-i", arg0}, args...)
	args = append(args, arg1)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	cmd.Stderr = err

	if ok0 {
		cmd.Stdin = in
	} else {
		cmd.Stdin = nil
	}

	if ok1 {
		cmd.Stdout = out
	} else {
		cmd.Stdout = io.Discard
	}

	return cmd
}

func pipe(itf interface{}, fd int) (string, bool) {
	switch impl := itf.(type) {
	case *os.File:
		return impl.Name(), false
	default:
		return fmt.Sprintf("pipe:%d", fd), true
	}
}
