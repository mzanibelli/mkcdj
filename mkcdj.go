package mkcdj

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Track is an audio track.
type Track struct {
	Path string  `json:"path"`
	Hash string  `json:"hash"`
	BPM  float64 `json:"bpm"`
}

// String implements fmt.Stringer for Track.
func (t Track) String() string {
	return fmt.Sprintf("%0.f - %s", t.BPM, filepath.Base(t.Path))
}

// Preset is a BPM range preset.
type Preset struct {
	Min float64
	Max float64
}

var (
	DNB     = Preset{165, 179.99}
	Jungle  = Preset{155, 164.99}
	Dubstep = Preset{135, 144.99}
	Garage  = Preset{128, 134.99}
	House   = Preset{115, 127.99}
	Default = Preset{80, 114.99}
)

// Range returns the BPM range as used for parameter interpolation in the
// analyze pipeline.
func (p Preset) Range() (string, string) {
	min, max := math.Round(p.Min), math.Round(p.Max)
	return fmt.Sprintf("%0.f", min), fmt.Sprintf("%0.f", max)
}

var presets = map[string]Preset{
	"dnb":     DNB,
	"jungle":  Jungle,
	"dubstep": Dubstep,
	"garage":  Garage,
	"house":   House,
	"default": Default,
}

// PresetFromName returns a BPM range preset from its name.
func PresetFromName(name string) (Preset, error) {
	if p, ok := presets[name]; ok {
		return p, nil
	}
	return Default, fmt.Errorf("unknown preset: %s", name)
}

// PresetFromBPM returns the BPM range matching a given value.
func PresetFromBPM(bpm float64) (Preset, error) {
	for _, p := range presets {
		if p.Min <= bpm && bpm <= p.Max {
			return p, nil
		}
	}
	return Default, fmt.Errorf("unknown BPM range for value: %2.f", bpm)
}

// Playlist is a DJ playlist.
type Playlist struct {
	collection Repository
	analyze    Pipeline
	convert    Pipeline
}

// Repository holds the track collection.
type Repository interface {
	Save(v interface{}) error
	Load(v interface{}) error
}

// Pipeline is an external Unix pipeline.
type Pipeline interface {
	Command(context.Context) *exec.Cmd
}

// PipelineFunc is a function implementation of Pipeline.
type PipelineFunc func(context.Context) *exec.Cmd

// Command implements Pipeline for PipelineFunc.
func (f PipelineFunc) Command(ctx context.Context) *exec.Cmd {
	return f(ctx)
}

// New returns a new Playlist.
func New(opts ...Option) *Playlist {
	a := new(Playlist)
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Option is an option of the BPM analyzer.
type Option func(*Playlist)

// WithRepository configures the repository used to persist data.
func WithRepository(r Repository) Option {
	return func(a *Playlist) {
		a.collection = r
	}
}

// WithAnalyzeFunc configures the shell command used to compute BPM data.
func WithAnalyzeFunc(f func(context.Context) *exec.Cmd) Option {
	return func(a *Playlist) {
		a.analyze = PipelineFunc(f)
	}
}

// WithConvertFunc configures the shell command used to convert final files.
func WithConvertFunc(f func(context.Context) *exec.Cmd) Option {
	return func(a *Playlist) {
		a.convert = PipelineFunc(f)
	}
}

// List pretty-prints the current playlist.
func (a *Playlist) List(out io.Writer) error {
	tracks := make([]Track, 0)

	// Load the existing collection.
	if err := a.collection.Load(&tracks); err != nil {
		return err
	}

	// Sort collection by BPM.
	sort.SliceStable(tracks, func(i, j int) bool {
		return tracks[i].BPM < tracks[j].BPM
	})

	for _, t := range tracks {
		// Check if the file is found.
		var status string
		if _, err := os.Stat(t.Path); err == nil {
			status = "[ok]"
		} else {
			status = "[ko]"
		}

		// Print out a line for the track.
		line := fmt.Sprintf("%s %s", status, t)
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}

	return nil
}

// Analyze computes the BPM of an audio file.
// It analyses the file at the given path passing min and max as BPM boundaries
// to the external program.
func (a *Playlist) Analyze(ctx context.Context, path string) error {
	tracks := make([]Track, 0)

	// Load the existing collection.
	if err := a.collection.Load(&tracks); err != nil {
		return err
	}

	// Ensure all steps of the process use an absolute file path, especially upon save.
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}

	wg := new(sync.WaitGroup)
	wg.Add(2)

	hc, bc, ec := make(chan string, 1), make(chan float64, 1), make(chan error, 2)

	// Hash the file. This will be used to avoid duplicates in the collection as
	// well as speed up some operations.
	go func() {
		defer wg.Done()
		hash, err := hash(abs)
		hc <- hash
		ec <- err
	}()

	// Compute the BPM analysis from the given shell pipeline. Convert the command
	// output to a float64 value.
	go func() {
		defer wg.Done()
		bpm, err := analyze(ctx, abs, a.analyze)
		bc <- bpm
		ec <- err
	}()

	wg.Wait()

	close(hc)
	close(bc)
	close(ec)

	// Handle any error that occurred during hash or BPM analysis steps.
	for err := range ec {
		if err != nil {
			return err
		}
	}

	// Create the Track struct from freshly computed data.
	track := Track{Path: abs, Hash: <-hc, BPM: <-bc}

	// Check if the same track was already in our collection and update it with the
	// new version if it is found.
	var found bool
	for i := range tracks {
		if tracks[i].Hash == track.Hash {
			tracks[i] = track
			found = true
			break
		}
	}

	// Otherwise append the new track to the collection.
	if !found {
		tracks = append(tracks, track)
	}

	// Persist the final collection.
	return a.collection.Save(&tracks)
}

func (a *Playlist) Compile(ctx context.Context, path string) error {
	tracks := make([]Track, 0)

	// Load the existing collection.
	if err := a.collection.Load(&tracks); err != nil {
		return err
	}

	// Create the directory for the final playlist.
	dir, err := os.MkdirTemp(filepath.Clean(path), "mkcdj-*")
	if err != nil {
		return err
	}

	const numWorkers = 4

	// Initialize scheduling tools.
	wg := new(sync.WaitGroup)
	input := make(chan Track, numWorkers)
	sink := make(chan error, len(tracks))

	wg.Add(numWorkers + 1)

	// This returns the full path of the destination WAV file according to the
	// previously created destination directory as well as a given filepath.
	mkPath := func(t Track) string {
		var subdir string

		p, err := PresetFromBPM(t.BPM)
		if p == Default || err != nil {
			subdir = "default"
		} else {
			min, max := p.Range()
			subdir = fmt.Sprintf("%s - %s", min, max)
		}

		base, ext := filepath.Base(t.Path), filepath.Ext(t.Path)
		name := base[:len(base)-len(ext)]

		return filepath.Join(dir, subdir, fmt.Sprintf("%0.f - %s.wav", t.BPM, name))
	}

	// Start the workers that will handle file conversions.
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			for t := range input {
				sink <- convert(ctx, t.Path, mkPath(t), a.convert)
			}
		}()
	}

	// Feed the input channel with every track in the collection.
	go func() {
		defer wg.Done()
		defer close(input)
		for _, t := range tracks {
			input <- t
		}
	}()

	wg.Wait()

	close(sink)

	// Handle errors.
	for err := range sink {
		if err != nil {
			return err
		}
	}

	return nil
}

func hash(path string) (string, error) {
	fd, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer fd.Close()

	h := sha256.New()
	if _, err := io.Copy(h, fd); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func analyze(ctx context.Context, path string, p Pipeline) (float64, error) {
	fd, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer fd.Close()

	buf := bytes.NewBuffer(nil)

	if err := run(ctx, p, fd, buf); err != nil {
		return 0, err
	}

	res, err := strconv.ParseFloat(strings.TrimSpace(buf.String()), 64)
	if err != nil {
		return 0, err
	}

	return res, nil
}

func convert(ctx context.Context, src, dst string, p Pipeline) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	return run(ctx, p, in, out)
}

func run(parent context.Context, p Pipeline, stdin io.Reader, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	cmd := p.Command(ctx)

	cmd.Stdin, cmd.Stdout = stdin, stdout

	return cmd.Run()
}
