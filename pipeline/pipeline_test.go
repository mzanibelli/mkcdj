package pipeline_test

import (
	"mkcdj/pipeline"
	"testing"
)

func TestCheck(t *testing.T) {
	if err := pipeline.Check(); err != nil {
		t.Error(err)
	}
}

const skipMessage = "this test requires real audio files to be executed"

func TestAnalyze(t *testing.T) {
	t.Skip(skipMessage)
}

func TestConvert(t *testing.T) {
	t.Skip(skipMessage)
}

func TestWaveform(t *testing.T) {
	t.Skip(skipMessage)
}

func TestSpectrogram(t *testing.T) {
	t.Skip(skipMessage)
}
