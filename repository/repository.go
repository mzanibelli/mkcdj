// Package repository allows to persist and load data.
package repository

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// JSONFile saves and loads the whole collection from a single JSON file.
type JSONFile string

// Load loads data from the repository into the given struct.
func (repo JSONFile) Load(data interface{}) error {
	fd, err := os.Open(filepath.Clean(string(repo)))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer fd.Close()

	return json.NewDecoder(fd).Decode(data)
}

// Load saves data held by the given struct into the repository.
func (repo JSONFile) Save(data interface{}) error {
	fd, err := os.Create(filepath.Clean(string(repo)))
	if err != nil {
		return err
	}
	defer fd.Close()

	return json.NewEncoder(fd).Encode(data)
}
