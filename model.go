package core

import (
	"log"
	"os"
	"path/filepath"
)

/*Model TODO

TODO tracked has bad performance once very large - replace with struct? Size
argument in make seems not to make a difference.
*/
type Model struct {
	root       string
	tracked    map[string]bool
	objinfo    map[string]staticinfo
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

/*
staticinfo stores all information that Tinzenite must keep between calls to
m.Update(). This includes the object ID and version for reapplication, plus
the content hash if required for file content changes detection.
*/
type staticinfo struct {
	Identification string
	Version        map[string]int
	Directory      bool
	Content        string
}

/*LoadModel todo*/
func LoadModel(path string) (*Model, error) {
	if !IsTinzenite(path) {
		return nil, ErrNotTinzenite
	}
	/*TODO load if model available*/
	// what follows is the code for a NEW model
	m := &Model{
		root:    path,
		tracked: make(map[string]bool),
		objinfo: make(map[string]staticinfo)}
	// build first version (note that updatechan can't possibly be set already, so we won't spam UpdateMessages)
	err := m.Update()
	if err != nil {
		return nil, err
	}
	return m, nil
}

/*
Update the complete model state.
TODO: check concurrency allowances?
*/
func (m *Model) Update() error {
	current, err := m.populate()
	var removed, created []string
	if err != nil {
		return err
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
		}
	}
	// CREATED - any remaining paths are yet untracked in m.tracked
	for path := range current {
		created = append(created, path)
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
	return nil
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

/*
getInfo creates the Objectinfo for the given path, so long as the path is
contained in m.tracked. Directories are NOT traversed!
*/
func (m *Model) getInfo(path string) (*Objectinfo, error) {
	_, exists := m.tracked[path]
	if !exists {
		return nil, ErrUntracked
	}
	stat, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	// TODO lots still to do here!
	object := &Objectinfo{Path: path}
	if stat.IsDir() {
		object.directory = true
	}
	// TODO apply staticinfo!
	return object, ErrUnsupported
}

/*
populate a map[path] for the m.root path. Applies the root Matcher if provided.
*/
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
model, not touching m.tracked. NEVER call this method outside of m.Update()!
*/
func (m *Model) apply(op Operation, path string) {
	notify := false
	var infoToNotify staticinfo
	switch op {
	case Create:
		notify = true
		// fetch all values we'll need to store
		id, err := newIdentifier()
		if err != nil {
			log.Println(err.Error())
			return
		}
		stat, err := os.Lstat(path)
		if err != nil {
			log.Println(err.Error())
			return
		}
		hash := ""
		if !stat.IsDir() {
			hash, err = contentHash(path)
			if err != nil {
				log.Println(err.Error())
				return
			}
		}
		stin := staticinfo{
			Identification: id,
			Version:        make(map[string]int),
			Directory:      stat.IsDir(),
			Content:        hash}
		m.objinfo[path] = stin
		infoToNotify = stin
	case Modify:
		stin, ok := m.objinfo[path]
		if !ok {
			log.Println("staticinfo lookup failed!")
			return
		}
		// no need for further work here
		if stin.Directory {
			return
		}
		hash, err := contentHash(path)
		if err != nil {
			log.Println(err.Error())
			return
		}
		// if same --> no changes, so done
		if hash == stin.Content {
			return
		}
		// otherwise a change has happened
		notify = true
		// update
		stin.Content = hash
		m.objinfo[path] = stin
		// TODO update version
		infoToNotify = stin
	case Remove:
		notify = true
		var ok bool
		infoToNotify, ok = m.objinfo[path]
		if !ok {
			log.Println("staticinfo lookup failed!")
			notify = false
		}
		delete(m.objinfo, path)
	default:
		log.Printf("Unimplemented %s for now!\n", op)
	}
	// send the update message
	if notify && m.updatechan != nil {
		/*TODO select with default --> lost message? but we loose every update
		after the first... hm*/
		m.updatechan <- UpdateMessage{Operation: op, Object: infoToNotify}
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
