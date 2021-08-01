// Package quality computes a score based on the presence of high-end frequencies.
// Is parses output from sox(1) freq module.
package quality

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// Threshold is the expected minimum score for a good quality. Based on
// empirical study of high-quality audio files.
const Threshold = 0.3

const (
	lowCut  = 16000
	highCut = 20000
)

// Parse parses the output of SoX "stat -freq" module to compute a quality
// score based on the average gain in high frequencies.
func Parse(r io.Reader) (float64, error) {
	var lt, lc float64
	var ht, hc float64

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())

		freq, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return 0, err
		}

		gain, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			return 0, err
		}

		if freq < lowCut {
			continue
		}

		lt += gain
		lc++

		if freq < highCut {
			continue
		}

		ht += gain
		hc++
	}

	return (ht / hc) / (lt / lc), nil
}
