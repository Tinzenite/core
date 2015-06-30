package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

/*
Model TODO
*/
type model struct {
	Root       string
	SelfID     string
	Tracked    map[string]bool
	Objinfo    map[string]staticinfo
	updatechan chan UpdateMessage
}

/*
CreateModel creates a new model at the specified path for the given peer id.
*/
func createModel(root, peerid string) (*model, error) {
	if !IsTinzenite(root) {
		return nil, ErrNotTinzenite
	}
	m := &model{
		Root:    root,
		Tracked: make(map[string]bool),
		Objinfo: make(map[string]staticinfo),
		SelfID:  peerid}
	// ensure that off line updates are caught (note that updatechan won't notify these)
	err := m.Update()
	if err != nil {
		// explicitely return nil because it is a severe error
		return nil, err
	}
	return m, nil
}

/*
LoadModel loads or creates a model for the given path, depending whether a
model.json exists for it already. Also immediately builds the model for the
first time and stores it.
*/
func loadModel(root string) (*model, error) {
	if !IsTinzenite(root) {
		return nil, ErrNotTinzenite
	}
	var m *model
	data, err := ioutil.ReadFile(root + "/" + TINZENITEDIR + "/" + LOCAL + "/" + MODELJSON)
	if err != nil {
		return nil, err
	}
	// load as json
	err = json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
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
disk at the end. Heavy concurrency used here.

TODO Get concurrency to work here. Last time I had trouble with the Objinfo map.
*/
func (m *model) Update() error {
	if m.Tracked == nil || m.Objinfo == nil {
		return ErrNilInternalState
	}
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
ApplyUpdateMessage takes an update message and applies it to the model. Should
be called after the file operation has been applied but before the next update!
*/
func (m *model) ApplyUpdateMessage(msg *UpdateMessage) error {
	// NOTE: NO YOU CANNOT USE m.apply() FOR THIS!
	path := createPath(m.Root, msg.Object.Path)
	switch msg.Operation {
	case Create:
		log.Printf("Create %s\n", path.FullPath())
		// sanity check if the object already exists locally
		_, ok := m.Tracked[path.FullPath()]
		if ok {
			log.Println("Object already exists locally! Can not apply create!")
			return errConflict
		}
		// ensure file has been written
		_, err := os.Lstat(path.FullPath())
		if err != nil {
			return err
		}
		// build staticinfo
		stin, err := createStaticInfo(path.FullPath(), m.SelfID)
		if err != nil {
			return err
		}
		// apply version
		/*TODO this is fishy, not correct yet - currently we loose the external version!*/
		stin.Version = msg.Object.Version
		// add obj to local model
		m.Tracked[path.FullPath()] = true
		m.Objinfo[path.FullPath()] = *stin
	case Modify:
		log.Printf("Modify %s\n", path.FullPath())
	case Remove:
		log.Printf("Remove %s\n", path.FullPath())
		/*TODO multiple peer logic*/
	default:
		log.Printf("Unknown operation in UpdateMessage: %s\n", msg.Operation)
		return ErrUnsupported
	}
	// finally store so that we know that the update has been applied
	return m.store()
}

/*
Register the channel over which UpdateMessage can be received. Tinzenite will
only ever write to this channel, never read.
*/
func (m *model) Register(v chan UpdateMessage) {
	m.updatechan = v
}

/*
Read builds the complete Objectinfo representation of this model to its full
depth. Incredibly fast because we only link objects based on the current state
of the model: hashes etc are not recalculated.
*/
func (m *model) Read() (*ObjectInfo, error) {
	var allObjs sortable
	rpath := createPathRoot(m.Root)
	// getting all Objectinfos is very fast because the staticinfo already exists for all of them
	for fullpath := range m.Tracked {
		obj, err := m.getInfo(rpath.Apply(fullpath))
		if err != nil {
			log.Println(err.Error())
			continue
		}
		allObjs = append(allObjs, obj)
	}
	// sort so that we can linearly run through based on the path
	sort.Sort(allObjs)
	// build the tree!
	root := allObjs[0]
	/*build tree recursively*/
	m.fillInfo(root, allObjs)
	return root, nil
}

/*
store the model to disk in the correct directory.
*/
func (m *model) store() error {
	path := m.Root + "/" + TINZENITEDIR + "/" + LOCAL + "/" + MODELJSON
	jsonBinary, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, jsonBinary, FILEPERMISSIONMODE)
}

/*
getInfo creates the Objectinfo for the given path, so long as the path is
contained in m.Tracked. Directories are NOT traversed!
*/
func (m *model) getInfo(path *relativePath) (*ObjectInfo, error) {
	_, exists := m.Tracked[path.FullPath()]
	if !exists {
		log.Printf("Error: %s\n", path.FullPath())
		return nil, ErrUntracked
	}
	// get staticinfo
	stin, exists := m.Objinfo[path.FullPath()]
	if !exists {
		log.Printf("Error: %s\n", path.FullPath())
		return nil, ErrUntracked
	}
	stat, err := os.Lstat(path.FullPath())
	if err != nil {
		return nil, err
	}
	// build object
	object := &ObjectInfo{
		Identification: stin.Identification,
		Name:           path.LastElement(),
		Path:           path.Subpath(),
		Shadow:         false,
		Version:        stin.Version}
	if stat.IsDir() {
		object.directory = true
		object.Content = ""
	} else {
		object.directory = false
		object.Content = stin.Content
	}
	return object, nil
}

/*
fillInfo takes an Objectinfo and a list of candidates and recursively fills its
Objects slice. If root is a file it simply returns root.
*/
func (m *model) fillInfo(root *ObjectInfo, all []*ObjectInfo) *ObjectInfo {
	if !root.directory {
		// this may be an error, check later
		return root
	}
	rpath := createPath(m.Root, root.Path)
	for _, obj := range all {
		if obj == root {
			// skip self
			continue
		}
		path := rpath.Apply(m.Root + "/" + obj.Path)
		if path.Depth() != rpath.Depth()+1 {
			// ignore any out of depth objects
			continue
		}
		if !strings.Contains(path.FullPath(), rpath.FullPath()) {
			// not in my directory
			log.Println("Not in mine!") // leave this line until you figure out why it never runs into this...
			continue
		}
		// if reached the object is in our subdir, so add and recursively fill
		root.Objects = append(root.Objects, m.fillInfo(obj, all))
	}
	return root
}

/*
populate a map[path] for the m.root path. Applies the root Matcher if provided.

TODO: need to be capable of applying sub Matchers...
*/
func (m *model) populate() (map[string]bool, error) {
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
func (m *model) apply(op Operation, path string) {
	// whether to send an update on updatechan
	notify := false
	switch op {
	case Create:
		notify = true
		stin, err := createStaticInfo(path, m.SelfID)
		if err != nil {
			log.Println(err.Error())
			return
		}
		m.Objinfo[path] = *stin
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
		// if modtime still the same no need to hash again
		stat, err := os.Lstat(path)
		if err != nil {
			log.Println(err.Error())
			// Note that we don't return here because we can still continue without this check
		} else {
			if stat.ModTime() == stin.Modtime {
				return
			}
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
		stin.Version.Increase(m.SelfID)
		m.Objinfo[path] = stin
	case Remove:
		/*TODO: delete logic for multiple peers required!*/
		notify = true
		delete(m.Objinfo, path)
		// note: m.tracked is modified in m.Update(), don't touch it here!
	default:
		log.Printf("Unimplemented %s operation!\n", op)
	}
	// send the update message
	if notify && m.updatechan != nil {
		relPath := createPathRoot(m.Root).Apply(path)
		obj, err := m.getInfo(relPath)
		if err != nil {
			log.Printf("Error messaging update for %s!\n", path)
		}
		m.updatechan <- UpdateMessage{Operation: op, Object: *obj}
	}
}

/*TODO for now only lists all tracked files for debug*/
func (m *model) String() string {
	var list string
	for path := range m.Tracked {
		list += path + "\n"
	}
	return list
}
