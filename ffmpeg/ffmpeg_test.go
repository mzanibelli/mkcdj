package ffmpeg_test

import (
	"testing"
)

const skipMessage = "this test requires real audio files to be executed"

func TestAnalyze(t *testing.T)     { t.Skip(skipMessage) }
func TestConvert(t *testing.T)     { t.Skip(skipMessage) }
func TestWaveform(t *testing.T)    { t.Skip(skipMessage) }
func TestSpectrogram(t *testing.T) { t.Skip(skipMessage) }
