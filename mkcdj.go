package mkcdj

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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
	return fmt.Sprintf("[%s] [%.0f] %s", status(t), math.Round(t.BPM), filepath.Base(t.Path))
}

// Preset is a BPM range preset.
type Preset struct {
	Min float64
	Max float64
}

var (
	DNB     = Preset{165, 179.99}
	Jungle  = Preset{148, 164.99}
	Dubstep = Preset{135, 147.99}
	Garage  = Preset{128, 134.99}
	House   = Preset{115, 127.99}
	Default = Preset{1, 200}
)

// Internal list used for lookup.
var presets = map[string]Preset{
	"dnb":     DNB,
	"jungle":  Jungle,
	"dubstep": Dubstep,
	"garage":  Garage,
	"house":   House,
}

// Range returns the BPM range as used for parameter interpolation in the
// analyze pipeline.
func (p Preset) Range() (string, string) {
	min, max := math.Round(p.Min), math.Round(p.Max)
	return fmt.Sprintf("%.0f", min), fmt.Sprintf("%.0f", max)
}

// PresetFromName returns list BPM range preset from its name.
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
	return Default, fmt.Errorf("unknown BPM range for value: %.2f", bpm)
}

// Playlist is a DJ playlist.
type Playlist struct {
	collection Repository
	analyze    Pipeline
	convert    Pipeline
	waveform   Pipeline
	spectrum   Pipeline
	scanner    BPMScanner
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
	list := new(Playlist)
	for _, opt := range opts {
		opt(list)
	}
	return list
}

// Option is an option of the BPM analyzer.
type Option func(*Playlist)

// WithRepository configures the repository used to persist data.
func WithRepository(r Repository) Option {
	return func(list *Playlist) {
		list.collection = r
	}
}

// WithPipeline configures one of the pipelines.
func WithPipeline(name string, f func(context.Context) *exec.Cmd) Option {
	return func(list *Playlist) {
		switch name {
		case "analyze":
			list.analyze = PipelineFunc(f)
		case "convert":
			list.convert = PipelineFunc(f)
		case "waveform":
			list.waveform = PipelineFunc(f)
		case "spectrum":
			list.spectrum = PipelineFunc(f)
		default:
			panic("unknown pipeline")
		}
	}
}

// BPMScanner scans raw f32le data for BPM given a range.
type BPMScanner interface {
	Scan(r io.Reader, min, max float64) (float64, error)
}

// BPMScanFunc is a function implementation of BPMScanner.
type BPMScanFunc func(r io.Reader, min, max float64) (float64, error)

// Scan implements BPMScanner for BPMScanFunc.
func (f BPMScanFunc) Scan(r io.Reader, min, max float64) (float64, error) {
	return f(r, min, max)
}

// WithBPMScanFunc configures the BPM scanner.
func WithBPMScanFunc(f func(r io.Reader, min, max float64) (float64, error)) Option {
	return func(list *Playlist) {
		list.scanner = BPMScanFunc(f)
	}
}

// List pretty-prints the current playlist.
func (list *Playlist) List(out io.Writer) error {
	tracks := make([]Track, 0)

	// Load the existing collection.
	if err := list.collection.Load(&tracks); err != nil {
		return err
	}

	// Sort collection by BPM.
	sort.SliceStable(tracks, func(i, j int) bool {
		return tracks[i].BPM < tracks[j].BPM
	})

	// Print all the tracks and their metadata.
	for _, t := range tracks {
		if _, err := fmt.Fprintln(out, t); err != nil {
			return err
		}
	}

	return nil
}

// Files prints all the absolute file paths, one per line.
func (list *Playlist) Files(out io.Writer) error {
	tracks := make([]Track, 0)

	// Load the existing collection.
	if err := list.collection.Load(&tracks); err != nil {
		return err
	}

	// Print all the files.
	for _, t := range tracks {
		if _, err := fmt.Fprintln(out, t.Path); err != nil {
			return err
		}
	}

	return nil
}

// Prune remove files that are not a their reported location anymore.
// It is based on the status() function, so this could have more criteria in
// the near future.
func (list *Playlist) Prune() error {
	tracks := make([]Track, 0)

	// Load the existing collection.
	if err := list.collection.Load(&tracks); err != nil {
		return err
	}

	// Cleanup tracks with an error status (file not found...).
	clean := make([]Track, 0)
	for i := range tracks {
		if status(tracks[i]) != fail {
			clean = append(clean, tracks[i])
		} else {
			log.Println(tracks[i])
		}
	}

	// Persist the final collection.
	return list.collection.Save(&clean)
}

// Analyze adds a track to the playlist and computes its BPM.
func (list *Playlist) Analyze(ctx context.Context, path string, preset Preset) error {
	tracks := make([]Track, 0)

	// Load the existing collection.
	if err := list.collection.Load(&tracks); err != nil {
		return err
	}

	// Ensure all steps of the process use an absolute file path, especially upon save.
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}

	// Compute the Track.
	track, err := track(ctx, abs, preset, list.analyze, list.scanner)
	if err != nil {
		return err
	}

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

	log.Println(track)

	// Persist the final collection.
	return list.collection.Save(&tracks)
}

// Refresh re-analyzes all tracks in the playlist.
func (list *Playlist) Refresh(ctx context.Context) error {
	tracks := make([]Track, 0)

	// Load the existing collection.
	if err := list.collection.Load(&tracks); err != nil {
		return err
	}

	fresh := make([]Track, 0)

	// Each job will spawn two goroutines.
	var n = runtime.NumCPU() / 2

	log.Println("[workers]", n)

	// Protect the new track collection from concurrent writes.
	var mu sync.Mutex

	// Use the BPM range determined from previous analysis.
	do := func(t Track) error {
		p, _ := PresetFromBPM(t.BPM)

		t, err := track(ctx, t.Path, p, list.analyze, list.scanner)
		if err != nil {
			return err
		}

		log.Println(t)

		mu.Lock()
		defer mu.Unlock()
		fresh = append(fresh, t)

		return nil
	}

	// Re-analyze the whole collection.
	if err := each(n, tracks, do); err != nil {
		return err
	}

	// Persist the final collection.
	return list.collection.Save(&fresh)
}

// Compile converts all files to a common format and exports them in the given
// directory classified by BPM.
func (list *Playlist) Compile(ctx context.Context, path string) error {
	tracks := make([]Track, 0)

	// Load the existing collection.
	if err := list.collection.Load(&tracks); err != nil {
		return err
	}

	// Create the directory for the final playlist.
	dir, err := os.MkdirTemp(filepath.Clean(path), "mkcdj-*")
	if err != nil {
		return err
	}

	// Limit concurrency to avoid bottlenecks while exporting to disk.
	// Each job will spawn three FFMPEG processes.
	var n = runtime.NumCPU() / 3

	log.Println("[workers]", n)

	// This function is the core processing step of each worker.
	// Internally, this handles file conversions, images generations...
	do := func(t Track) error {
		return convert(ctx, dir, t, list.convert, list.waveform, list.spectrum)
	}

	// Process each track.
	if err := each(n, tracks, do); err != nil {
		return err
	}

	log.Println("[done]", dir)

	return nil
}

func each(size int, tracks []Track, do func(t Track) error) error {
	// Initialize scheduling tools.
	wg := new(sync.WaitGroup)
	jobs := make(chan Track, size)
	sink := make(chan error, size)

	teardown := func() {
		close(jobs)
		wg.Wait()
		close(sink)
	}

	wg.Add(size)

	// Start workers. Run until the input channel is closed.
	for i := 0; i < size; i++ {
		go func() {
			defer wg.Done()
			for t := range jobs {
				sink <- do(t)
			}
		}()
	}

	var once sync.Once
	defer once.Do(teardown)

	// Feed the input channel with every track in the collection. Once done,
	// closing the input channel will stop the workers. We wait for workers to finish
	// before closing the error sink.
	go func() {
		defer once.Do(teardown)
		for _, t := range tracks {
			jobs <- t
		}
	}()

	// Even if we don't finish reading this channel, buffering will ensure we can
	// flush workers before returning early in case of error.
	for err := range sink {
		if err != nil {
			return err
		}
	}

	return nil
}

func rename(t Track) string {
	var subdir string

	switch p, err := PresetFromBPM(t.BPM); {
	case err == nil:
		min, max := p.Range()
		subdir = fmt.Sprintf("%s - %s", min, max)
	default:
		subdir = "default"
	}

	base, ext := filepath.Base(t.Path), filepath.Ext(t.Path)
	name := base[:len(base)-len(ext)]
	path := fmt.Sprintf("%.0f - %s", math.Round(t.BPM), name)

	return filepath.Join(subdir, path)
}

func track(ctx context.Context, path string, preset Preset, p Pipeline, s BPMScanner) (Track, error) {
	wg := new(sync.WaitGroup)
	wg.Add(2)

	hc, bc := make(chan string, 1), make(chan float64, 1)
	sink := make(chan error, 2)

	go func() {
		defer wg.Done()
		hash, err := hash(path)
		hc <- hash
		sink <- err
	}()

	go func() {
		defer wg.Done()
		bpm, err := analyze(ctx, path, preset, p, s)
		bc <- bpm
		sink <- err
	}()

	wg.Wait()

	close(hc)
	close(bc)

	close(sink)

	for err := range sink {
		if err != nil {
			return Track{}, err
		}
	}

	return Track{Path: path, Hash: <-hc, BPM: <-bc}, nil
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

func analyze(ctx context.Context, path string, preset Preset, p Pipeline, s BPMScanner) (float64, error) {
	fd, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer fd.Close()

	buf := bytes.NewBuffer(nil)

	if err := run(ctx, p, bufio.NewReader(fd), buf); err != nil {
		return 0, err
	}

	return s.Scan(buf, preset.Min, preset.Max)
}

func convert(ctx context.Context, root string, t Track, c, w, s Pipeline) error {
	log.Println(t)

	wg, sink := new(sync.WaitGroup), make(chan error, 3)
	wg.Add(3)

	dst := func(dir, suffix string) string {
		return filepath.Join(dir, rename(t)+suffix)
	}

	audio := filepath.Join(root, "audio")
	waves := filepath.Join(root, "waveforms")
	specs := filepath.Join(root, "spectrograms")

	go func() {
		defer wg.Done()
		sink <- build(ctx, t.Path, dst(audio, wav), c)
	}()

	go func() {
		defer wg.Done()
		sink <- build(ctx, t.Path, dst(waves, png), w)
	}()

	go func() {
		defer wg.Done()
		sink <- build(ctx, t.Path, dst(specs, png), s)
	}()

	wg.Wait()

	close(sink)

	for err := range sink {
		if err != nil {
			return err
		}
	}

	return nil
}

func build(ctx context.Context, src, dst string, p Pipeline) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		return fmt.Errorf("about to overwrite: %s", dst)
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	r, w := bufio.NewReader(in), bufio.NewWriter(out)

	if err := run(ctx, p, r, w); err != nil {
		return err
	}

	return w.Flush()
}

func run(parent context.Context, p Pipeline, stdin io.Reader, stdout io.Writer) error {
	const pipelineTimeout = 1 * time.Minute

	ctx, cancel := context.WithTimeout(parent, pipelineTimeout)
	defer cancel()

	cmd := p.Command(ctx)

	stderr := bytes.NewBuffer(nil)

	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr

	line, err := stderr.ReadString(0x0A)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}

	if message := strings.TrimSpace(line); message != "" {
		log.Println(message)
	}

	return cmd.Run()
}

const (
	// Status strings.
	good = "good"
	warn = "warn"
	fail = "fail"

	// File extensions.
	wav  = ".wav"
	flac = ".flac"
	png  = ".png"
)

func status(t Track) string {
	ext := filepath.Ext(t.Path)
	switch _, err := os.Stat(t.Path); {
	case err != nil:
		return fail
	case ext != wav && ext != flac:
		return warn
	default:
		return good
	}
}
