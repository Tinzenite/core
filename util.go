package core

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"os"
)

/*
IsTinzenite checks whether a given path is indeed a valid directory
*/
// TODO detect incomplete dir (no connected peers, etc)
func IsTinzenite(dirpath string) bool {
	_, err := os.Stat(dirpath + "/.tinzenite")
	if err == nil {
		return true
	}
	// NOTE: object may exist but we may not have permission to access it: in that case
	//       we consider it unaccessible and thus return false
	return false
}

func saveContext(context *Context) error {
	JSON, err := json.MarshalIndent(context, "", "    ")
	if err != nil {
		return err
	}
	dir := context.Path + "/.tinzenite"
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
	data, err := ioutil.ReadFile(dirpath + "/.tinzenite/temp.json")
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
	data := make([]byte, 10)
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

func makeDirectory(path string) error {
	return os.Mkdir(path, 0777)
}

func makeFile(path string) error {
	return ErrUnsupported
}
