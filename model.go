package core

import (
	"os"
	"path/filepath"
)

/*TODO: make everything private*/

/*Model todo*/
type Model struct {
	root string
	/*
	   TODO bad performance once very large - replace with struct? Size argument
	   in make seems not to make a difference.
	*/
	tracked map[string]bool
}

/*Objectinfo todo*/
type Objectinfo struct {
	directory      bool
	Identification string
	Name           string
	Path           string
	Shadow         bool
	Version        map[string]int
	// Objects        []*Objectinfo `json:",omitempty"`
	Content string `json:",omitempty"`
}

/*LoadModel todo*/
func LoadModel(path string) (*Model, error) {
	if !IsTinzenite(path) {
		return nil, ErrNotTinzenite
	}
	m := &Model{
		root:    path,
		tracked: make(map[string]bool)}
	return m, m.populate()
}

/*Update todo*/
func (m *Model) Update() (bool, error) {
	/*TODO*/
	return false, ErrUnsupported
}

/*Register todo*/
func (m *Model) Register(v chan Objectinfo) {
	/*TODO*/
}

/*Read todo*/
func (m *Model) Read() (*Objectinfo, error) {
	/*TODO*/
	return nil, ErrUnsupported
}

func (m *Model) store() error {
	return ErrUnsupported
}

func (m *Model) populate() error {
	match, err := CreateMatcher(m.root)
	if err != nil {
		return err
	}
	filepath.Walk(m.root, func(subpath string, stat os.FileInfo, inerr error) error {
		// ignore on match
		if match.Ignore(subpath) {
			// SkipDir is okay even if file
			if stat.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		m.tracked[subpath] = true
		return nil
	})
	return nil
}

/*TODO for now only lists all tracked files*/
func (m *Model) String() string {
	var list string
	for path := range m.tracked {
		list += path + "\n"
	}
	return list
}
