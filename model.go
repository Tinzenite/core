package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

/*Model TODO

TODO Tracked has bad performance once very large - replace with struct? Size
argument in make seems not to make a difference.
*/
type Model struct {
	Root       string
	Tracked    map[string]bool
	Objinfo    map[string]staticinfo
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

/*
LoadModel loads or creates a model for the given path, depending whether a
model.json exists for it already. Also immediately builds the model for the
first time and stores it.
*/
func LoadModel(path string) (*Model, error) {
	if !IsTinzenite(path) {
		return nil, ErrNotTinzenite
	}
	var m *Model
	data, err := ioutil.ReadFile(path + "/" + TINZENITEDIR + "/" + LOCAL + "/" + MODELJSON)
	if err != nil {
		// if error we must create a new one
		m = &Model{
			Root:    path,
			Tracked: make(map[string]bool),
			Objinfo: make(map[string]staticinfo)}
	} else {
		// load as json
		err = json.Unmarshal(data, &m)
		if err != nil {
			return nil, err
		}
	}
	// ensure that off line updates are caught (note that updatechan won't notify these)
	err = m.Update()
	if err != nil {
		// explicitely return nil because it is a severe error
		return nil, err
	}
	return m, nil
}

/*
Update the complete model state. Will if successful try to store the model to
disk at the end.

TODO: check concurrency allowances?
*/
func (m *Model) Update() error {
	current, err := m.populate()
	var removed, created []string
	if err != nil {
		return err
	}
	for path := range m.Tracked {
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
	// update m.Tracked
	for _, path := range removed {
		delete(m.Tracked, path)
		m.apply(Remove, path)
	}
	for _, path := range created {
		m.Tracked[path] = true
		m.apply(Create, path)
	}
	// finally also store the model for future loads.
	return m.store()
}

/*
Register the channel over which UpdateMessage can be received. Tinzenite will
only ever write to this channel, never read.
*/
func (m *Model) Register(v chan UpdateMessage) {
	m.updatechan = v
}

/*
Read TODO

Should return the JSON representation of this directory
*/
func (m *Model) Read() (*Objectinfo, error) {
	/*TODO this can be massively parallelized: call getInfo for all objects
	with multiple go routines, then construct the tree afterwards.*/
	return nil, ErrUnsupported
}

/*
store the model to disk in the correct directory.
*/
func (m *Model) store() error {
	dir := m.Root + "/" + TINZENITEDIR + "/" + LOCAL
	err := makeDirectory(dir)
	if err != nil {
		return err
	}
	jsonBinary, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dir+"/"+MODELJSON, jsonBinary, FILEPERMISSIONMODE)
}

/*
getInfo creates the Objectinfo for the given path, so long as the path is
contained in m.Tracked. Directories are NOT traversed!
*/
func (m *Model) getInfo(path string) (*Objectinfo, error) {
	/*TODO incomplete yet*/
	_, exists := m.Tracked[path]
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
	match, err := CreateMatcher(m.Root)
	if err != nil {
		return nil, err
	}
	tracked := make(map[string]bool)
	filepath.Walk(m.Root, func(subpath string, stat os.FileInfo, inerr error) error {
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
	// method elsewhere (for the current structure on m.Update())
	return tracked, nil
}

/*
Apply changes to the internal model state. This method does the true logic on the
model, not touching m.Tracked. NEVER call this method outside of m.Update()!
*/
func (m *Model) apply(op Operation, path string) {
	// whether to send an update on updatechan
	notify := false
	// object for notify
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
		m.Objinfo[path] = stin
		infoToNotify = stin
	case Modify:
		stin, ok := m.Objinfo[path]
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
		m.Objinfo[path] = stin
		// TODO update version
		infoToNotify = stin
	case Remove:
		/*TODO: delete logic for multiple peers required!*/
		notify = true
		var ok bool
		infoToNotify, ok = m.Objinfo[path]
		if !ok {
			log.Println("staticinfo lookup failed!")
			notify = false
		}
		delete(m.Objinfo, path)
	default:
		log.Printf("Unimplemented %s for now!\n", op)
	}
	// send the update message
	if notify && m.updatechan != nil {
		/*TODO async select with default --> lost message? but we loose every update
		after the first... hm*/
		m.updatechan <- UpdateMessage{Operation: op, Object: infoToNotify}
	}
}

/*TODO for now only lists all tracked files for debug*/
func (m *Model) String() string {
	var list string
	for path := range m.Tracked {
		list += path + "\n"
	}
	return list
}
