package mkcdj_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mkcdj"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPresets(t *testing.T) {
	t.Run("it should load the preset by its name", func(t *testing.T) {
		p, err := mkcdj.PresetFromName("dnb")
		noerr(t, err)
		assert(t, "dnb", p.Name)
	})

	t.Run("it should return an error and the default preset if the name is not found", func(t *testing.T) {
		p, err := mkcdj.PresetFromName("foo")
		assert(t, true, err != nil)
		assert(t, "default", p.Name)
	})

	t.Run("it should load the preset by its bpm range, using the narrowest encountered range", func(t *testing.T) {
		p, err := mkcdj.PresetFromBPM(174)
		noerr(t, err)
		assert(t, "dnb", p.Name)
	})

	t.Run("it should return an error and the default preset for unsupported BPM ranges", func(t *testing.T) {
		p, err := mkcdj.PresetFromBPM(20)
		assert(t, true, err != nil)
		assert(t, "default", p.Name)
	})
}

func TestSerialization(t *testing.T) {
	t.Run("it should unserialize and serialize a playlist", func(t *testing.T) {
		data := `[{"path":"/foo","hash":"bar","preset":"dnb","bpm":100}]`

		var tracks []mkcdj.Track
		noerr(t, json.Unmarshal([]byte(data), &tracks))
		assert(t, "/foo", tracks[0].Path)
		assert(t, "bar", tracks[0].Hash)
		assert(t, "dnb", tracks[0].Preset.Name)
		assert(t, 100, tracks[0].BPM)

		got, err := json.Marshal(&tracks)
		noerr(t, err)
		assert(t, data, string(got))
	})

	t.Run("it should use the default preset if the track preset is empty", func(t *testing.T) {
		data := `[{"path":"/foo","hash":"bar","preset":"","bpm":100}]`

		var tracks []mkcdj.Track
		noerr(t, json.Unmarshal([]byte(data), &tracks))
		assert(t, "default", tracks[0].Preset.Name)
	})
}

func TestAnalyze(t *testing.T) {
	SUT, params, teardown := setup(t)
	t.Cleanup(teardown)

	// Do the analysis twice to check duplication.
	noerr(t, SUT.Analyze(context.Background(), params.SourceFilePath, mkcdj.Presets[0]))
	noerr(t, SUT.Analyze(context.Background(), params.SourceFilePath, mkcdj.Presets[0]))

	tracks := loadPlaylist(t, params.PlaylistFilePath)

	assert(t, 1, len(tracks))
	assert(t, params.SourceFilePath, tracks[0].Path)
	assert(t, "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03", tracks[0].Hash)
	assert(t, 100, tracks[0].BPM)
}

func TestRefresh(t *testing.T) {
	SUT, params, teardown := setup(t)
	t.Cleanup(teardown)

	noerr(t, SUT.Refresh(context.Background()))

	tracks := loadPlaylist(t, params.PlaylistFilePath)

	assert(t, 1, len(tracks))
	assert(t, params.SourceFilePath, tracks[0].Path)
	assert(t, "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03", tracks[0].Hash)
	assert(t, 100, tracks[0].BPM)
}

func TestCompile(t *testing.T) {
	SUT, params, teardown := setup(t)
	t.Cleanup(teardown)

	noerr(t, SUT.Compile(context.Background(), params.OutDirPath))

	files := listFiles(t, params.OutDirPath)

	t.Log(files)

	base, ext := filepath.Base(params.SourceFilePath), filepath.Ext(params.SourceFilePath)

	want := fmt.Sprintf("100 - %s", base[:len(base)-len(ext)])

	assert(t, 3, len(files))
	assert(t, want+".wav", filepath.Base(files[0]))
	assert(t, "default", filepath.Base(filepath.Dir(files[0])))

	checkFile(t, params.OutDirPath, filepath.Dir(files[0]), want+".wav")
	checkFile(t, params.OutDirPath, filepath.Dir(files[1]), want+".png")
	checkFile(t, params.OutDirPath, filepath.Dir(files[2]), want+".png")
}

type params struct {
	SourceFilePath   string
	OutDirPath       string
	PlaylistFilePath string
}

func setup(t *testing.T) (*mkcdj.Playlist, params, func()) {
	t.Helper()

	dir, err := os.MkdirTemp(os.TempDir(), "mkcdj-*")
	noerr(t, err)

	fd, err := os.CreateTemp(dir, "mkcdj-source-*.flac")
	noerr(t, err)
	_, err = fmt.Fprintln(fd, "hello")
	noerr(t, err)
	noerr(t, fd.Close())

	tracks := []mkcdj.Track{mkcdj.Track{
		Path:   fd.Name(),
		Hash:   "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03",
		BPM:    100,
		Preset: mkcdj.Presets[0],
	}}

	payload, err := json.Marshal(tracks)
	noerr(t, err)

	playlist := filepath.Join(os.TempDir(), "/mkcdj.json")
	noerr(t, os.WriteFile(playlist, payload, 0666))

	SUT := mkcdj.New(
		mkcdj.WithRepository(playlist),
		mkcdj.WithPipeline(mkcdj.Convert, writeOk),
		mkcdj.WithPipeline(mkcdj.Analyze, writeOk),
		mkcdj.WithPipeline(mkcdj.Waveform, writeOk),
		mkcdj.WithPipeline(mkcdj.Spectrum, writeOk),
		mkcdj.WithBPMScanFunc(stubBPMScanner),
	)

	res := params{
		SourceFilePath:   fd.Name(),
		OutDirPath:       dir,
		PlaylistFilePath: playlist,
	}

	return SUT, res, func() { os.RemoveAll(dir) }
}

func loadPlaylist(t *testing.T, path string) []mkcdj.Track {
	t.Helper()
	tracks := make([]mkcdj.Track, 0)
	data, err := os.ReadFile(path)
	noerr(t, err)
	noerr(t, json.Unmarshal(data, &tracks))
	return tracks
}

func listFiles(t *testing.T, path string) []string {
	files, err := fs.Glob(os.DirFS(path), "mkcdj-*/*/*/*")
	noerr(t, err)
	return files
}

func checkFile(t *testing.T, components ...string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(components...))
	noerr(t, err)
	assert(t, "ok", strings.TrimSpace(string(content)))
}

func assert[T comparable](t *testing.T, want, got T) {
	t.Helper()
	if want != got {
		t.Errorf("want: %v, got: %v", want, got)
	}
}

func noerr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

var writeOk = mkcdj.PipelineFunc(stubCmd)

func stubCmd(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	_, err := stdout.Write([]byte("ok"))
	return err
}

func stubBPMScanner(r io.Reader, min, max float64) (float64, error) { return 100, nil }
