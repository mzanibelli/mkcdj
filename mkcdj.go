package mkcdj

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"math"
	"os"
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
	Dubstep = Preset{138, 147.99}
	Garage  = Preset{130, 137.99}
	House   = Preset{115, 129.99}
	Default = Preset{060, 114.99}
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
	rounded := math.Round(bpm*100) / 100
	for _, p := range presets {
		if p.Min <= rounded && rounded <= p.Max {
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
	Run(context.Context, io.Reader, io.Writer, io.Writer) error
}

// PipelineFunc is a function implementation of Pipeline.
type PipelineFunc func(context.Context, io.Reader, io.Writer, io.Writer) error

// Command implements Pipeline for PipelineFunc.
func (f PipelineFunc) Run(ctx context.Context, in io.Reader, out, err io.Writer) error {
	return f(ctx, in, out, err)
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
func WithPipeline(name string, p Pipeline) Option {
	return func(list *Playlist) {
		switch name {
		case "analyze":
			list.analyze = p
		case "convert":
			list.convert = p
		case "waveform":
			list.waveform = p
		case "spectrum":
			list.spectrum = p
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

	if err := list.collection.Load(&tracks); err != nil {
		return err
	}

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

	if err := list.collection.Load(&tracks); err != nil {
		return err
	}

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
	old := make([]Track, 0)

	if err := list.collection.Load(&old); err != nil {
		return err
	}

	new := make([]Track, 0)
	for i := range old {
		if status(old[i]) != fail {
			new = append(new, old[i])
		} else {
			log.Println(old[i])
		}
	}

	return list.collection.Save(&new)
}

// Analyze adds a track to the playlist and computes its BPM.
func (list *Playlist) Analyze(ctx context.Context, path string, preset Preset) error {
	tracks := make([]Track, 0)

	if err := list.collection.Load(&tracks); err != nil {
		return err
	}

	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}

	track, err := track(ctx, abs, preset, list.analyze, list.scanner)
	if err != nil {
		return err
	}

	var found bool
	for i := range tracks {
		if tracks[i].Hash == track.Hash {
			tracks[i] = track
			found = true
			break
		}
	}

	if !found {
		tracks = append(tracks, track)
	}

	log.Println(track)

	order(tracks)
	return list.collection.Save(&tracks)
}

// Refresh re-analyzes all tracks in the playlist.
func (list *Playlist) Refresh(ctx context.Context) error {
	old := make([]Track, 0)

	if err := list.collection.Load(&old); err != nil {
		return err
	}

	// Each job will spawn two goroutines (hash and BPM analysis).
	var n = runtime.NumCPU() / 2

	log.Println("[workers]", n)

	out, new, wg := make(chan Track, n), make([]Track, 0), new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for t := range out {
			new = append(new, t)
		}
	}()

	do := func(t Track) error {
		p, _ := PresetFromBPM(t.BPM)

		t, err := track(ctx, t.Path, p, list.analyze, list.scanner)
		if err != nil {
			return err
		}

		log.Println(t)

		out <- t

		return nil
	}

	if err := each(n, old, do); err != nil {
		close(out)
		wg.Wait()
		return err
	}

	close(out)

	wg.Wait()

	order(new)
	return list.collection.Save(&new)
}

// Compile converts all files to a common format and exports them in the given
// directory classified by BPM.
func (list *Playlist) Compile(ctx context.Context, path string) error {
	tracks := make([]Track, 0)

	if err := list.collection.Load(&tracks); err != nil {
		return err
	}

	dir, err := os.MkdirTemp(filepath.Clean(path), "mkcdj-*")
	if err != nil {
		return err
	}

	// Each job will spawn three FFMPEG processes.
	var n = runtime.NumCPU() / 3

	log.Println("[workers]", n)

	do := func(t Track) error {
		return convert(ctx, dir, t, list.convert, list.waveform, list.spectrum)
	}

	if err := each(n, tracks, do); err != nil {
		return err
	}

	log.Println("[done]", dir)

	return nil
}

func order(tracks []Track) {
	sort.SliceStable(tracks, func(i, j int) bool {
		b1, b2 := math.Round(tracks[i].BPM), math.Round(tracks[j].BPM)
		if b1 == b2 {
			f1, f2 := filepath.Base(tracks[i].Path), filepath.Base(tracks[j].Path)
			return strings.Compare(f1, f2) == -1
		}
		return b1 < b2
	})
}

func each(size int, tracks []Track, do func(t Track) error) error {
	wg := new(sync.WaitGroup)
	jobs := make(chan Track, size)
	sink := make(chan error, size)

	teardown := func() {
		close(jobs)
		wg.Wait()
		close(sink)
	}

	wg.Add(size)

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

	go func() {
		defer once.Do(teardown)
		for _, t := range tracks {
			jobs <- t
		}
	}()

	for err := range sink {
		if err != nil {
			return err
		}
	}

	return nil
}

func rename(t Track) string {
	var subdir string

	switch p, _ := PresetFromBPM(t.BPM); {
	case p != Default:
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

	return run(ctx, p, in, out)
}

func run(parent context.Context, p Pipeline, stdin io.Reader, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(parent, 1*time.Minute)
	defer cancel()

	stderr := bytes.NewBuffer(nil)

	err := p.Run(ctx, stdin, stdout, stderr)

	line, _ := stderr.ReadString(0x0A)
	if message := strings.TrimSpace(line); message != "" {
		log.Println(message)
	}

	return err
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
