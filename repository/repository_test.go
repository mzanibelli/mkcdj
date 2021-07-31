package repository_test

import (
	"mkcdj/repository"
	"os"
	"testing"
)

const pattern = "mkcdj-*"

type Foo struct {
	Foo string `json:"foo"`
}

func TestRepository(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), pattern)
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(dir)

	fd, err := os.CreateTemp(dir, pattern)
	if err != nil {
		t.Error(err)
	}

	defer os.Remove(fd.Name())
	defer fd.Close()

	SUT := repository.JSONFile(fd.Name())

	const val = "hello"

	a := new(Foo)
	a.Foo = val

	if err := SUT.Save(a); err != nil {
		t.Error(err)
	}

	b := new(Foo)
	if err := SUT.Load(b); err != nil {
		t.Error(err)
	}

	if b.Foo != val {
		t.Errorf("want: %s, got: %s", val, b.Foo)
	}
}
