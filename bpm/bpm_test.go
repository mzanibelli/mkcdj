package bpm_test

import (
	"fmt"
	"mkcdj/bpm"
	"os"
	"testing"
)

func TestBPM(t *testing.T) {
	fd, err := os.Open("./testdata/track.dat")
	if err != nil {
		t.Error(err)
	}
	defer fd.Close()

	got, err := bpm.Scan(fd, 115, 128)
	if err != nil {
		t.Error(err)
	}

	assert(t, "118", fmt.Sprintf("%.0f", got))
}

func assert(t *testing.T, want, got string) {
	if want != got {
		t.Errorf("want: %s, got: %s", want, got)
	}
}
