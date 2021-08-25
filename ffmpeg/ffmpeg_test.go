package ffmpeg_test

import (
	"bytes"
	"context"
	"io"
	"mkcdj/ffmpeg"
	"os"
	"testing"
	"time"
)

func TestFFMPEG(t *testing.T) {
	t.Run("analyze", run(ffmpeg.F32LE))
	t.Run("convert", run(ffmpeg.AudioOut))
	t.Run("waveform", run(ffmpeg.PNGWaveform))
	t.Run("spectrum", run(ffmpeg.PNGSpectrum))
}

func run(f func(context.Context, io.Reader, io.Writer, io.Writer) error) func(t *testing.T) {
	return func(t *testing.T) {
		in, err := os.Open("./testdata/track.wav")
		if err != nil {
			t.Error(err)
		}
		defer in.Close()

		stderr := bytes.NewBuffer(nil)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := f(ctx, in, io.Discard, stderr); err != nil {
			t.Log(stderr.String())
			t.Error(err)
		}
	}
}
