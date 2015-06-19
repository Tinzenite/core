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

/*
IsTinzenite checks whether a given path is indeed a valid directory
*/
// TODO detect incomplete dir (no connected peers, etc)
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

func saveContext(context *Context) error {
	JSON, err := json.MarshalIndent(context, "", "    ")
	if err != nil {
		return err
	}
	dir := context.Path + "/" + TINZENITEDIR
	err = os.Mkdir(dir, 0777)
	if err != nil {
		return err
	}
	file, err := os.Create(dir + "/temp.json")
	if err != nil {
		return err
	}
	_, err = file.Write(JSON)
	return err
}

// dirpath MUST be only to main dir!
func loadContext(dirpath string) (*Context, error) {
	data, err := ioutil.ReadFile(dirpath + "/" + TINZENITEDIR + "/temp.json")
	if err != nil {
		return nil, err
	}
	var context *Context
	err = json.Unmarshal(data, &context)
	if err != nil {
		return nil, err
	}
	return context, err
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

func makeDirectories(root string, subdirs ...string) error {
	for _, path := range subdirs {
		err := makeDirectory(root + "/" + path)
		if err != nil {
			return err
		}
	}
	return nil
}
