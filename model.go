package core

import (
	"os"
	"path/filepath"
)

/*TODO: make everything private*/

/*Model todo*/
type Model struct {
	root    string
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
	filepath.Walk(m.root, func(subpath string, stat os.FileInfo, inerr error) error {
		m.tracked[subpath] = true
		return nil
	})
	return nil
}

func (m *Model) String() string {
	var list string
	for path := range m.tracked {
		list += path + "\n"
	}
	return list
}
