package core

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"os"
	"os/user"
	"strings"
)

type relativePath struct {
	Root    string
	Subpath string
}

func createPath(rootpath string) *relativePath {
	rootpath = strings.TrimSuffix(rootpath, "/")
	return &relativePath{Root: rootpath,
		Subpath: ""}
}

func (r *relativePath) FullPath() string {
	if r.Subpath == "" {
		return r.Root
	}
	return r.Root + "/" + r.Subpath
}

func (r *relativePath) Down(step string) *relativePath {
	step = strings.Trim(step, "/")
	newPath := &relativePath{Root: r.Root, Subpath: r.Subpath}
	if r.Subpath == "" {
		newPath.Subpath = step
	} else {
		newPath.Subpath = r.Subpath + "/" + step
	}
	return newPath
}

func (r *relativePath) Up() *relativePath {
	newPath := &relativePath{Root: r.Root, Subpath: r.Subpath}
	index := strings.LastIndex(r.Subpath, "/")
	if index < 0 {
		return newPath
	}
	newPath.Subpath = r.Subpath[:index]
	return newPath
}

func (r *relativePath) LastElement() string {
	index := strings.LastIndex(r.FullPath(), "/")
	if index < 0 {
		return "/"
	}
	return r.FullPath()[index+1:]
}

func (r *relativePath) Apply(path string) *relativePath {
	newPath := &relativePath{Root: r.Root, Subpath: r.Subpath}
	if strings.HasPrefix(path, r.Root) {
		newPath.Subpath = strings.TrimPrefix(path, r.Root)
	}
	return newPath
}

func (r *relativePath) Validate() bool {
	_, err := os.Lstat(r.FullPath())
	if err != nil {
		return false
	}
	return true
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
