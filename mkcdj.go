package mkcdj

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
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
	"syscall"
	"time"
)

// Track is an audio track.
type Track struct {
	Path   string  `json:"path"`
	Hash   string  `json:"hash"`
	Preset Preset  `json:"preset"`
	BPM    float64 `json:"bpm"`
}

// String implements fmt.Stringer for Track.
func (t Track) String() string {
	return fmt.Sprintf("[%s] [%s] [%.0f] %s",
		status(t), t.Preset.Name, math.Round(t.BPM), filepath.Base(t.Path))
}

// Presets is the list of available presets.
// It must have at least one element being the default preset at index 0.
var Presets = [...]Preset{
	{"default", 40, 220}, // Largo to Prestissimo.

	{"dnb", 165, 179.99},
	{"jungle", 148, 164.99},
	{"dubstep", 138, 147.99},
	{"techno", 128, 137.99},
	{"house", 115, 129.99},
	{"hiphop", 60, 114.99},
	{"dub", 60, 89.99},
}

// Preset is a BPM range preset.
type Preset struct {
	Name string
	Min  float64
	Max  float64
}

// UnmarshalJSON implements json.Unmarshaler for Preset.
// If the preset is empty, the default preset is silently returned.
func (p *Preset) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}

	if name == "" {
		*p = Presets[0]
		return nil
	}

	var err error
	*p, err = PresetFromName(name)
	return err
}

// MarshalJSON implements json.Marshaler for Preset.
func (p *Preset) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Name)
}

// Range returns the BPM range as used for parameter interpolation in the
// analyze pipeline.
func (p Preset) Range() (string, string) {
	min, max := math.Round(p.Min), math.Round(p.Max)
	return fmt.Sprintf("%.0f", min), fmt.Sprintf("%.0f", max)
}

// PresetFromBPM returns the Preset with the narrowest BPM range matching the given value.
func PresetFromBPM(bpm float64) (Preset, error) {
	var match Preset

	rounded := math.Round(bpm*100) / 100
	for _, p := range Presets {
		// Skip non-matching ranges.
		if p.Min > rounded || rounded > p.Max {
			continue
		}

		// Automatically match the first encountered preset.
		if match.Name == "" {
			match = p
			continue
		}

		// Replace the match by the narrowest encountered range.
		if p.Max-p.Min < match.Max-match.Min {
			match = p
			continue
		}
	}

	if match.Name == "" {
		return Presets[0], fmt.Errorf("unknown BPM range for value: %.2f", bpm)
	}

	return match, nil
}

// PresetFromName returns list BPM range preset from its name.
func PresetFromName(name string) (Preset, error) {
	for _, p := range Presets {
		if p.Name == name {
			return p, nil
		}
	}
	return Presets[0], fmt.Errorf("unknown preset: %s", name)
}

// Playlist is a DJ playlist.
type Playlist struct {
	path      string
	pipelines [4]Pipeline
	scanner   BPMScanner
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
func WithRepository(path string) Option {
	return func(list *Playlist) {
		list.path = path
	}
}

// A codec is a way of transcoding the signal.
type codec int

const (
	Analyze  codec = iota // Prepare audio file for BPM analysis.
	Convert               // Convert to final format.
	Waveform              // Generate waveform picture.
	Spectrum              // Generate spectrogram picture.
)

// WithPipeline configures one of the pipelines.
func WithPipeline(c codec, p Pipeline) Option {
	return func(list *Playlist) {
		list.pipelines[c] = p
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
	return withJSONFile(list.path, func(tracks []Track) ([]Track, error) {
		for _, t := range tracks {
			if _, err := fmt.Fprintln(out, t); err != nil {
				return nil, err
			}
		}
		return tracks, nil
	})
}

// Files prints all the absolute file paths, one per line.
func (list *Playlist) Files(out io.Writer) error {
	return withJSONFile(list.path, func(tracks []Track) ([]Track, error) {
		for _, t := range tracks {
			if _, err := fmt.Fprintln(out, t.Path); err != nil {
				return nil, err
			}
		}
		return tracks, nil
	})
}

// Prune remove files that are not a their reported location anymore.
// It is based on the status() function, so this could have more criteria in
// the near future.
func (list *Playlist) Prune() error {
	return withJSONFile(list.path, func(old []Track) ([]Track, error) {
		tracks := make([]Track, 0)
		for i := range old {
			if status(old[i]) != fail {
				tracks = append(tracks, old[i])
			} else {
				log.Println(old[i])
			}
		}
		return tracks, nil
	})
}

// Analyze adds a track to the playlist and computes its BPM.
func (list *Playlist) Analyze(ctx context.Context, path string, preset Preset) error {
	return withJSONFile(list.path, func(tracks []Track) ([]Track, error) {
		abs, err := filepath.Abs(filepath.Clean(path))
		if err != nil {
			return nil, err
		}

		track, err := track(ctx, abs, preset, list.pipelines[Analyze], list.scanner)
		if err != nil {
			return nil, err
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

		return tracks, nil
	})
}

// Refresh re-analyzes all tracks in the playlist.
func (list *Playlist) Refresh(ctx context.Context) error {
	return withJSONFile(list.path, func(old []Track) ([]Track, error) {
		// Each job will spawn two goroutines (hash and BPM analysis).
		var n = runtime.NumCPU() / 2

		log.Println("[workers]", n)

		out, tracks, wg := make(chan Track, n), make([]Track, 0), new(sync.WaitGroup)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range out {
				tracks = append(tracks, t)
			}
		}()

		do := func(t Track) error {
			// Recompute the appropriate preset from the last known BPM. It allows to
			// change and move preset layout around freely.
			if t.Preset.Name == "" {
				t.Preset, _ = PresetFromBPM(t.BPM)
			}

			t, err := track(ctx, t.Path, t.Preset, list.pipelines[Analyze], list.scanner)
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
			return nil, err
		}

		close(out)

		wg.Wait()

		order(tracks)

		return tracks, nil
	})
}

// Compile converts all files to a common format and exports them in the given
// directory classified by BPM.
func (list *Playlist) Compile(ctx context.Context, path string) error {
	return withJSONFile(list.path, func(tracks []Track) ([]Track, error) {
		dir, err := os.MkdirTemp(filepath.Clean(path), "mkcdj-*")
		if err != nil {
			return nil, err
		}

		// Each job will spawn three FFMPEG processes.
		var n = runtime.NumCPU() / 3

		log.Println("[workers]", n)

		do := func(t Track) error {
			return convert(ctx, dir, t,
				list.pipelines[Convert],
				list.pipelines[Waveform],
				list.pipelines[Spectrum],
			)
		}

		if err := each(n, tracks, do); err != nil {
			return nil, err
		}

		log.Println("[done]", dir)

		return tracks, nil
	})
}

func order(tracks []Track) {
	sort.SliceStable(tracks, func(i, j int) bool {
		if p := strings.Compare(tracks[i].Preset.Name, tracks[j].Preset.Name); p != 0 {
			return p == -1
		}
		f1, f2 := filepath.Base(tracks[i].Path), filepath.Base(tracks[j].Path)
		return strings.Compare(f1, f2) == -1
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
	base, ext := filepath.Base(t.Path), filepath.Ext(t.Path)
	name := base[:len(base)-len(ext)]
	path := fmt.Sprintf("%.0f - %s", math.Round(t.BPM), name)
	return filepath.Join(t.Preset.Name, path)
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

	return Track{Path: path, Hash: <-hc, Preset: preset, BPM: <-bc}, nil
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

func withJSONFile[T any](path string, f func(data T) (T, error)) error {
	file, err := os.OpenFile(filepath.Clean(path), os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return fmt.Errorf("could not open file at path %q: %w", path, err)
	}
	defer file.Close()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("could not acquire exclusive lock on file at path %q: %w", path, err)
	}
	defer syscall.Flock(int(file.Fd()), syscall.LOCK_UN) //nolint:errcheck

	var data T
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return fmt.Errorf("could not decode data in file at path %q: %w", path, err)
	}

	replace, err := f(data)
	if err != nil {
		return err
	}

	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("could not truncate file at path %q: %w", path, err)
	}

	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("could not seek to beginning of file at path %q: %w", path, err)
	}

	return json.NewEncoder(file).Encode(replace)
}
