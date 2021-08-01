package quality_test

import (
	"mkcdj/quality"
	"os"
	"testing"
)

func TestQuality(t *testing.T) {
	fd, err := os.Open("./testdata/good.dat")
	if err != nil {
		t.Error(err)
	}
	defer fd.Close()

	got, err := quality.Parse(fd)
	if err != nil {
		t.Error(err)
	}

	if want := 0.333434414639791898427034766428; got != want {
		t.Errorf("want: %.30f, got: %.30f", want, got)
	}
}
