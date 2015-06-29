package core

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/user"
	"strings"
)

type relativePath struct {
	stack []string
	limit int
}

func createPathRoot(root string) *relativePath {
	r := relativePath{}
	list := strings.Split(root, "/")
	for _, element := range list {
		if element != "" {
			r.stack = append(r.stack, element)
		}
	}
	r.limit = len(r.stack)
	return &r
}

func createPath(root string, subpath string) *relativePath {
	r := createPathRoot(root)
	return r.Apply(root + "/" + subpath)
}

func (r *relativePath) FullPath() string {
	return "/" + strings.Join(r.stack, "/")
}

func (r *relativePath) LastElement() string {
	return r.stack[len(r.stack)-1]
}

func (r *relativePath) Subpath() string {
	return strings.Join(r.stack[r.limit:], "/")
}

func (r *relativePath) Apply(path string) *relativePath {
	if strings.HasPrefix(path, r.FullPath()) {
		relPath := createPathRoot(path)
		relPath.limit = r.limit
		return relPath
	}
	return &relativePath{limit: r.limit, stack: r.stack}
}

func (r *relativePath) Depth() int {
	return len(r.stack)
}

/*
IsTinzenite checks whether a given path is indeed a valid directory
*/
// TODO detect incomplete dir (no connected peers, etc) or write a validate method
func IsTinzenite(dirpath string) bool {
	_, err := os.Stat(dirpath + "/" + TINZENITEDIR)
	if err == nil {
		return true
	}
	// NOTE: object may exist but we may not have permission to access it: in that case
	//       we consider it unaccessible and thus return false
	return false
}

/*
PrettifyDirectoryList reads the directory.list file from the user's tinzenite
config directory and removes all invalid entries.
*/
// TODO rewrite this so that it accepts a string and then applies it if valid
//		while ensuring that the rest is valid
func PrettifyDirectoryList() error {
	user, err := user.Current()
	if err != nil {
		return err
	}
	path := user.HomeDir + "/.config/tinzenite/" + DIRECTORYLIST
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(bytes), "\n")
	writeList := map[string]bool{}
	for _, line := range lines {
		if IsTinzenite(line) {
			writeList[line] = true
		}
	}
	var newContents string
	for key := range writeList {
		newContents += key + "\n"
	}
	return ioutil.WriteFile(path, []byte(newContents), FILEPERMISSIONMODE)
}

func randomHash() (string, error) {
	data := make([]byte, RANDOMSEEDLENGTH)
	_, err := rand.Read(data)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, err = hash.Write(data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

/*
Creates a new random hash that is intended as identification strings for all
manner of different objects. Length is IDMAXLENGTH.
*/
func newIdentifier() (string, error) {
	data := make([]byte, RANDOMSEEDLENGTH)
	_, err := rand.Read(data)
	if err != nil {
		return "", err
	}
	hash := md5.New()
	_, err = hash.Write(data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil))[:IDMAXLENGTH], nil
}

func contentHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := md5.New()
	buf := make([]byte, CHUNKSIZE)
	// create hash
	for amount := CHUNKSIZE; amount == CHUNKSIZE; {
		amount, _ = file.Read(buf)
		// log.Printf("Read %d bytes", amount)
		hash.Write(buf)
	}
	// return hex representation
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func makeDirectory(path string) error {
	err := os.MkdirAll(path, FILEPERMISSIONMODE)
	// TODO this doesn't seem to work... why not?
	if err == os.ErrExist {
		return nil
	}
	// either successful or true error
	return err
}

/*
makeDirectories creates a number of directories in the given root path. Useful
if a complete directory tree has to be built at once.
*/
func makeDirectories(root string, subdirs ...string) error {
	for _, path := range subdirs {
		err := makeDirectory(root + "/" + path)
		if err != nil {
			return err
		}
	}
	return nil
}

/*
toxPeerDump stores the self peer information along with the tox binary data
required for it to work.
*/
type toxPeerDump struct {
	SelfPeer *Peer
	ToxData  []byte
}

/*
loadToxDump loads the toxPeerDump file for the local Tinzenite directory.
*/
func loadToxDump(root string) (*toxPeerDump, error) {
	data, err := ioutil.ReadFile(root + "/" + TINZENITEDIR + "/" + LOCAL + "/" + SELFPEERJSON)
	if err != nil {
		return nil, err
	}
	toxPeerDump := &toxPeerDump{}
	err = json.Unmarshal(data, toxPeerDump)
	if err != nil {
		return nil, err
	}
	return toxPeerDump, nil
}

/*
store the toxPeerDump to the directory.
*/
func (t *toxPeerDump) store(root string) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(root+"/"+TINZENITEDIR+"/"+LOCAL+"/"+SELFPEERJSON, data, FILEPERMISSIONMODE)
}

// sortable allows sorting Objectinfos by path.
type sortable []*objectInfo

func (s sortable) Len() int {
	return len(s)
}

func (s sortable) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sortable) Less(i, j int) bool {
	// path are sorted alphabetically all by themselves! :D
	return s[i].Path < s[j].Path
}
