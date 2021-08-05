// Package bpm computes the BPM of an audio file.
// The code is a simplified, slightly optimized, cleaned up version of
// github.com/benjojo/bpm, which is in turn a port of bpm-tools.
package bpm

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/rand"
)

const (
	Rate     = 44100
	Interval = 128
	Samples  = 1024
	Steps    = 1024
	X        = 8
	Y        = 512
)

// Scan returns the BPM of audio data from a Reader containing f32le samples.
// The BPM detection is between the given range.
func Scan(r io.Reader, min, max float64) (float64, error) {
	nrg, err := energy(r)
	if err != nil {
		return 0, err
	}
	return scan(nrg, min, max), nil
}

func energy(r io.Reader) ([]float32, error) {
	res := make([]float32, 0)

	var v, n float64

	for {
		var f float32

		switch err := binary.Read(r, binary.LittleEndian, &f); {
		case errors.Is(err, io.EOF):
			return res, nil
		case err != nil:
			return nil, err
		}

		z := math.Abs(float64(f))
		if z > v {
			v += (z - v) / X
		} else {
			v -= (v - z) / Y
		}

		n++
		if n == Interval {
			n, res = 0, append(res, float32(v))
		}
	}
}

func scan(nrg []float32, min, max float64) float64 {
	imin := bpmToInterval(min)
	imax := bpmToInterval(max)
	step := (imin - imax) / float64(Steps)

	height, trough := math.Inf(0), math.NaN()

	for interval := imax; interval <= imin; interval += step {
		var t float64

		for s := 0; s < Samples; s++ {
			t += autodifference(nrg, interval)
		}

		if t < height {
			trough = interval
			height = t
		}
	}

	return intervalToBpm(trough)
}

var (
	beats   = [...]float64{-32, -16, -8, -4, -2, -1, 1, 2, 4, 8, 16, 32}
	nobeats = [...]float64{-0.5, -0.25, 0.25, 0.5}
)

func autodifference(nrg []float32, interval float64) float64 {
	//nolint:gosec
	mid := rand.Float64() * float64(len(nrg))

	v := sample(nrg, mid)

	var diff, total float64

	for n := 0; n < (len(beats) / 2); n++ {
		y := sample(nrg, mid+beats[n]*interval)
		w := 1.0 / math.Abs(beats[n])
		diff += w * math.Abs(y-v)
		total += w
	}

	for n := 0; n < (len(nobeats) / 2); n++ {
		y := sample(nrg, mid+nobeats[n]*interval)
		w := math.Abs(nobeats[n])
		diff -= w * math.Abs(y-v)
		total += w
	}

	return diff / total
}

func sample(nrg []float32, offset float64) float64 {
	n := math.Floor(offset)
	if n >= 0.0 && n < float64(len(nrg)) {
		return float64(nrg[int(n)])
	}
	return 0.0
}

func bpmToInterval(bpm float64) float64 {
	beatsPerSecond := bpm / 60
	samplesPerBeat := Rate / beatsPerSecond
	return samplesPerBeat / Interval
}

func intervalToBpm(interval float64) float64 {
	samplesPerBeat := interval * Interval
	beatsPerSecond := Rate / samplesPerBeat
	return beatsPerSecond * 60
}
