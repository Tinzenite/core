package core

import (
	"log"
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
	/*TODO load if model available*/
	m := &Model{
		root:    path,
		tracked: make(map[string]bool)}
	tracked, err := m.populate()
	if err != nil {
		return nil, err
	}
	m.tracked = tracked
	return m, nil
}

/*Update todo*/
func (m *Model) Update() (bool, error) {
	current, err := m.populate()
	updated := false
	if err != nil {
		return false, err
	}
	for path := range m.tracked {
		_, ok := current[path]
		if ok {
			// remove if ok
			/*TODO here check if CONTENT different*/
			delete(current, path)
		} else {
			// REMOVED
			log.Println("REMOVED: " + path)
			updated = true
		}
	}
	// CREATED
	for path := range current {
		log.Println("CREATED: " + path)
		updated = true
	}
	return updated, nil
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

func (m *Model) getInfo(path string) (*Objectinfo, error) {
	_, exists := m.tracked[path]
	if !exists {
		return nil, ErrUntracked
	}
	/*TODO:
	- build object
	- what about children? Does this method only return empty dir and files?
	- dirs with content can be filled on Read(), right? Only need to place the
	  pointers correctly then.
	*/
	return nil, nil
}

func (m *Model) populate() (map[string]bool, error) {
	match, err := CreateMatcher(m.root)
	if err != nil {
		return nil, err
	}
	tracked := make(map[string]bool)
	filepath.Walk(m.root, func(subpath string, stat os.FileInfo, inerr error) error {
		// ignore on match
		if match.Ignore(subpath) {
			// SkipDir is okay even if file
			if stat.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		tracked[subpath] = true
		return nil
	})
	// doesn't directly assign to m.tracked on purpose so that we can reuse this
	// method elsewhere
	return tracked, nil
}

/*TODO for now only lists all tracked files*/
func (m *Model) String() string {
	var list string
	for path := range m.tracked {
		list += path + "\n"
	}
	return list
}
