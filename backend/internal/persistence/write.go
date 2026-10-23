// Package persistence provides the durable replacement used by app-owned state.
package persistence

import (
	"os"
	"path/filepath"
)

func Write(path string, b []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err == nil {
		d, e := os.Open(filepath.Dir(path))
		if e == nil {
			err = d.Sync()
			d.Close()
		} else {
			err = e
		}
	}
	return err
}
