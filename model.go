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
CreateModel creates a new model at the specified path for the given peer id. Will
not immediately update, must be explicitely called.
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
	return m, nil
}

/*
LoadModel loads or creates a model for the given path, depending whether a
model.json exists for it already.
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
	return m, nil
}

/*
Update the complete model state.
*/
func (m *model) Update() error {
	return m.PartialUpdate(m.Root)
}

/*
PartialUpdate of the model state.

TODO Get concurrency to work here. Last time I had trouble with the Objinfo map.
*/
func (m *model) PartialUpdate(scope string) error {
	if m.Tracked == nil || m.Objinfo == nil {
		return ErrNilInternalState
	}
	current, err := m.populateMap()
	var removed, created []string
	if err != nil {
		return err
	}
	// we'll need this for every create* op, so create only once:
	relPath := createPathRoot(m.Root)
	// now: compare old tracked with new version
	for path := range m.Tracked {
		// ignore if not in partial update path
		if !strings.HasPrefix(path, scope) {
			continue
		}
		_, ok := current[path]
		if ok {
			// paths that still exist must only be checked for MODIFY
			delete(current, path)
			m.applyModify(relPath.Apply(path), nil)
		} else {
			// REMOVED - paths that don't exist anymore have been removed
			removed = append(removed, path)
		}
	}
	// CREATED - any remaining paths are yet untracked in m.tracked
	for path := range current {
		// ignore if not in partial update path
		if !strings.HasPrefix(path, scope) {
			continue
		}
		created = append(created, path)
	}
	// update m.Tracked
	for _, path := range removed {
		m.applyRemove(relPath.Apply(path))
	}
	for _, path := range created {
		// nil for version because new local object
		m.applyCreate(relPath.Apply(path), nil)
	}
	// finally also store the model for future loads.
	return m.Store()
}

/*
ApplyUpdateMessage takes an update message and applies it to the model. Should
be called after the file operation has been applied but before the next update!
*/
/*TODO catch shadow files*/
func (m *model) ApplyUpdateMessage(msg *UpdateMessage) error {
	// NOTE: NO YOU CANNOT USE m.apply() FOR THIS!
	path := createPath(m.Root, msg.Object.Path)
	var err error
	switch msg.Operation {
	case OpCreate:
		err = m.applyCreate(path, msg.Object.Version)
	case OpModify:
		err = m.applyModify(path, msg.Object.Version)
	case OpRemove:
		err = m.applyRemove(path)
	default:
		log.Printf("Unknown operation in UpdateMessage: %s\n", msg.Operation)
		return ErrUnsupported
	}
	if err != nil {
		log.Println("Error on external apply update message!")
		return err
	}
	// store updates to disk
	return m.Store()
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
func (m *model) Store() error {
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
populateMap for the m.root path with all file and directory contents, with the
matcher applied if applicable.
*/
func (m *model) populateMap() (map[string]bool, error) {
	return m.partialPopulateMap(m.Root)
}

/*
partialPopulateMap for the given path with all file and directory contents within
the given path, with the matcher applied if applicable.
*/
func (m *model) partialPopulateMap(path string) (map[string]bool, error) {
	relPath := createPathRoot(m.Root).Apply(path)
	master, err := createMatcher(relPath.Rootpath())
	if err != nil {
		return nil, err
	}
	tracked := make(map[string]bool)
	filepath.Walk(relPath.FullPath(), func(subpath string, stat os.FileInfo, inerr error) error {
		// resolve matcher
		/*FIXME thie needlessly creates a lot of potential duplicates*/
		match := master.Resolve(relPath.Apply(subpath))
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
applyCreate applies a create operation to the local model given that the file
exists.
*/
func (m *model) applyCreate(path *relativePath, version version) error {
	// ensure file has been written
	if !fileExists(path.FullPath()) {
		return errIllegalFileState
	}
	// sanity check if the object already exists locally
	_, ok := m.Tracked[path.FullPath()]
	if ok {
		log.Println("Object already exists locally! Can not apply create!")
		return errConflict
	}
	// NOTE: we don't explicitely check m.Objinfo because we'll just overwrite it if already exists
	// build staticinfo
	stin, err := createStaticInfo(path.FullPath(), m.SelfID)
	if err != nil {
		return err
	}
	// apply version if given (external create) otherwise keep default one
	if version != nil {
		stin.Version = version
	}
	// add obj to local model
	m.Tracked[path.FullPath()] = true
	m.Objinfo[path.FullPath()] = *stin
	m.notify(OpCreate, path)
	return nil
}

/*
applyModify checks for modifications and if valid applies them to the local model.
Conflicts will result in deletion of the old file and two creations of both versions
of the conflict.
*/
func (m *model) applyModify(path *relativePath, version version) error {
	// ensure file has been written
	if !fileExists(path.FullPath()) {
		return errIllegalFileState
	}
	// sanity check
	_, ok := m.Tracked[path.FullPath()]
	if !ok {
		log.Println("Object doesn't exist locally!")
		return errIllegalFileState
	}
	// fetch stin
	stin, ok := m.Objinfo[path.FullPath()]
	if !ok {
		return errModelInconsitent
	}
	// if file hasn't changed we're done
	if !m.isModified(path.FullPath()) {
		return nil
	}
	// check for remote modifications
	if version != nil {
		/*TODO implement conflict behaviour!*/
		// detect conflict
		ver, ok := stin.Version.Valid(version, m.SelfID)
		if !ok {
			log.Println("Merge error!")
			/*TODO implement merge behavior in main.go*/
			return errConflict
		}
		// apply version update
		stin.Version = ver
	} else {
		// update version for local change
		stin.Version.Increase(m.SelfID)
	}
	// update hash and modtime
	err := stin.UpdateFromDisk(path.FullPath())
	if err != nil {
		return err
	}
	// apply updated
	m.Objinfo[path.FullPath()] = stin
	m.notify(OpModify, path)
	return nil
}

/*
applyRemove applies a remove operation.
*/
func (m *model) applyRemove(path *relativePath) error {
	/*TODO make sure this works for both local AND remote changes!*/
	// ensure file has been removed
	if fileExists(path.FullPath()) {
		return errIllegalFileState
	}
	/*TODO multiple peer logic*/
	delete(m.Tracked, path.FullPath())
	delete(m.Objinfo, path.FullPath())
	/*FIXME: we run into a problem: at this point the file is removed and untracked...*/
	m.notify(OpRemove, path)
	return nil
}

func (m *model) applyMerge() {

}

/*
isModified checks whether a file has been modified.
*/
func (m *model) isModified(path string) bool {
	stin, ok := m.Objinfo[path]
	if !ok {
		log.Println("staticinfo lookup failed!")
		return false
	}
	// no need for further work here
	if stin.Directory {
		return false
	}
	// if modtime still the same no need to hash again
	stat, err := os.Lstat(path)
	if err != nil {
		log.Println(err.Error())
		// Note that we don't return here because we can still continue without this check
	} else {
		if stat.ModTime() == stin.Modtime {
			return false
		}
	}
	hash, err := contentHash(path)
	if err != nil {
		log.Println(err.Error())
		return false
	}
	// if same --> no changes, so done
	if hash == stin.Content {
		return false
	}
	// otherwise a change has happened
	return true
}

/*
Notify the channel of the operation for the object at path.
*/
func (m *model) notify(op Operation, path *relativePath) {
	log.Printf("%s: %s\n", op, path.LastElement())
	if m.updatechan != nil {
		obj, err := m.getInfo(path)
		if err != nil {
			log.Printf("Error messaging update for %s!\n", path.FullPath())
			log.Println(err.Error())
			return
		}
		m.updatechan <- createUpdateMessage(op, *obj)
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
