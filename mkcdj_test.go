package mkcdj_test

import (
	"bytes"
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

func TestAnalyze(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "mkcdj-analyze-*")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(dir)

	fd, err := os.CreateTemp(dir, "mkcdj-analyze-source-*.flac")
	if err != nil {
		t.Error(err)
	}

	if _, err := fmt.Fprintln(fd, "hello"); err != nil {
		t.Error(err)
	}
	defer fd.Close()

	repo := new(fakeRepository)
	repo.buf = bytes.NewBufferString("[]")

	SUT := mkcdj.New(
		mkcdj.WithRepository(repo),
		mkcdj.WithPipeline(mkcdj.Analyze, writeOk),
		mkcdj.WithBPMScanFunc(stubBPMScanner),
	)

	ctx := context.Background()

	// Do the analysis twice to check duplication.
	if err := SUT.Analyze(ctx, fd.Name(), mkcdj.Default); err != nil {
		t.Error(err)
	}
	if err := SUT.Analyze(ctx, fd.Name(), mkcdj.Default); err != nil {
		t.Error(err)
	}

	t.Log(repo.buf.String())

	tracks := make([]mkcdj.Track, 0)

	if err := repo.Load(&tracks); err != nil {
		t.Error(err)
	}

	assert(t, "1", fmt.Sprint(len(tracks)))
	assert(t, fd.Name(), tracks[0].Path)
	assert(t, "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03", tracks[0].Hash)
	assert(t, "100", fmt.Sprint(tracks[0].BPM))
}

func TestRefresh(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "mkcdj-analyze-*")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(dir)

	fd, err := os.CreateTemp(dir, "mkcdj-analyze-source-*.flac")
	if err != nil {
		t.Error(err)
	}

	if _, err := fmt.Fprintln(fd, "hello"); err != nil {
		t.Error(err)
	}
	defer fd.Close()

	track := mkcdj.Track{
		Path: fd.Name(),
		Hash: "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03",
		BPM:  100,
	}

	tracks := []mkcdj.Track{track}

	repo := new(fakeRepository)
	repo.buf = bytes.NewBuffer(nil)

	if err := repo.Save(&tracks); err != nil {
		t.Error(err)
	}

	SUT := mkcdj.New(
		mkcdj.WithRepository(repo),
		mkcdj.WithPipeline(mkcdj.Analyze, writeOk),
		mkcdj.WithBPMScanFunc(stubBPMScanner),
	)

	ctx := context.Background()

	if err := SUT.Refresh(ctx); err != nil {
		t.Error(err)
	}

	t.Log(repo.buf.String())

	tracks = make([]mkcdj.Track, 0)

	if err := repo.Load(&tracks); err != nil {
		t.Error(err)
	}

	assert(t, "1", fmt.Sprint(len(tracks)))
	assert(t, fd.Name(), tracks[0].Path)
	assert(t, "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03", tracks[0].Hash)
	assert(t, "100", fmt.Sprint(tracks[0].BPM))
}

func TestCompile(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "mkcdj-compile-*")
	if err != nil {
		t.Error(err)
	}
	defer os.RemoveAll(dir)

	fd, err := os.CreateTemp(dir, "mkcdj-compile-source-*.flac")
	if err != nil {
		t.Error(err)
	}
	defer fd.Close()

	if _, err := fmt.Fprintln(fd, "hello"); err != nil {
		t.Error(err)
	}

	track := mkcdj.Track{
		Path: fd.Name(),
		Hash: "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03",
		BPM:  100,
	}

	tracks := []mkcdj.Track{track}

	repo := new(fakeRepository)
	repo.buf = bytes.NewBuffer(nil)

	if err := repo.Save(&tracks); err != nil {
		t.Error(err)
	}

	SUT := mkcdj.New(
		mkcdj.WithRepository(repo),
		mkcdj.WithPipeline(mkcdj.Convert, writeOk),
		mkcdj.WithPipeline(mkcdj.Waveform, writeOk),
		mkcdj.WithPipeline(mkcdj.Spectrum, writeOk),
	)

	if err := SUT.Compile(context.Background(), dir); err != nil {
		t.Error(err)
	}

	dirFS := os.DirFS(dir)

	files, err := fs.Glob(dirFS, "mkcdj-*/*/*/*")

	if err != nil {
		t.Error(err)
	}

	t.Log(files)

	base, ext := filepath.Base(fd.Name()), filepath.Ext(fd.Name())
	want := fmt.Sprintf("100 - %s", base[:len(base)-len(ext)])

	assert(t, "3", fmt.Sprint(len(files)))
	assert(t, want+".wav", filepath.Base(files[0]))
	assert(t, "default", filepath.Base(filepath.Dir(files[0])))

	checkFile(t, dir, filepath.Dir(files[0]), want+".wav")
	checkFile(t, dir, filepath.Dir(files[1]), want+".png")
	checkFile(t, dir, filepath.Dir(files[2]), want+".png")
}

func checkFile(t *testing.T, components ...string) {
	content, err := os.ReadFile(filepath.Join(components...))
	if err != nil {
		t.Error(err)
	}
	assert(t, "ok", strings.TrimSpace(string(content)))
}

func assert(t *testing.T, want, got string) {
	if want != got {
		t.Errorf("want: %s, got: %s", want, got)
	}
}

type fakeRepository struct{ buf *bytes.Buffer }

func (r *fakeRepository) Load(v interface{}) error { return json.NewDecoder(r.buf).Decode(v) }
func (r *fakeRepository) Save(v interface{}) error { return json.NewEncoder(r.buf).Encode(v) }

var writeOk = mkcdj.PipelineFunc(stubCmd)

func stubCmd(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	_, err := stdout.Write([]byte("ok"))
	return err
}

func stubBPMScanner(r io.Reader, min, max float64) (float64, error) { return 100, nil }
