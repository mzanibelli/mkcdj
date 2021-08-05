package ffmpeg_test

import (
	"bytes"
	"context"
	"io"
	"mkcdj/ffmpeg"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestFFMPEG(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("analyze", run(ffmpeg.F32LE(ctx)))
	t.Run("convert", run(ffmpeg.AudioOut(ctx)))
	t.Run("waveform", run(ffmpeg.PNGWaveform(ctx)))
	t.Run("spectrum", run(ffmpeg.PNGSpectrum(ctx)))
}

func run(cmd *exec.Cmd) func(t *testing.T) {
	return func(t *testing.T) {
		fd, err := os.Open("./testdata/track.wav")
		if err != nil {
			t.Error(err)
		}
		defer fd.Close()

		stderr := bytes.NewBuffer(nil)

		cmd.Stdin = fd
		cmd.Stdout = io.Discard
		cmd.Stderr = stderr

		err = cmd.Run()

		t.Log(stderr.String())

		if err != nil {
			t.Error(err)
		}
	}
}
