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
	tracked    map[string]bool
	updatechan chan UpdateMessage
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

/*
Update the complete model state.
TODO: check concurrency allowances?
*/
func (m *Model) Update() (bool, error) {
	current, err := m.populate()
	var removed, created []string
	updated := false
	if err != nil {
		return false, err
	}
	for path := range m.tracked {
		_, ok := current[path]
		if ok {
			// paths that still exist must only be checked for MODIFY
			delete(current, path)
			m.apply(Modify, path)
		} else {
			// REMOVED - paths that don't exist anymore have been removed
			removed = append(removed, path)
			updated = true
		}
	}
	// CREATED - any remaining paths are yet untracked in m.tracked
	for path := range current {
		created = append(created, path)
		updated = true
	}
	// update m.tracked
	for _, path := range removed {
		delete(m.tracked, path)
		m.apply(Remove, path)
	}
	for _, path := range created {
		m.tracked[path] = true
		m.apply(Create, path)
	}
	return updated, nil
}

/*
Register the channel over which UpdateMessage can be received. Tinzenite will
only ever write to this channel, never read.
*/
func (m *Model) Register(v chan UpdateMessage) {
	m.updatechan = v
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

/*
Apply changes to the internal model state. This method does the true logic on the
model, not touching m.tracked.
*/
func (m *Model) apply(op Operation, path string) {
	// TEMP: ignore
	if op == Modify {
		return
	}
	log.Printf("Doing %s on %s\n", op, path)
	if m.updatechan != nil {
		m.updatechan <- UpdateMessage{Operation: op}
	}
}

/*TODO for now only lists all tracked files*/
func (m *Model) String() string {
	var list string
	for path := range m.tracked {
		list += path + "\n"
	}
	return list
}
