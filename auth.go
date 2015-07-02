package core

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"io/ioutil"

	"golang.org/x/crypto/bcrypt"
)

/*
Authentication file.
*/
type Authentication struct {
	User    string
	Dirname string
	DirID   string
	Key     string
	block   cipher.Block
}

/*
loadAuthentication loads the auth.json file for the given Tinzenite directory.
*/
func loadAuthentication(path string) (*Authentication, error) {
	data, err := ioutil.ReadFile(path + "/" + TINZENITEDIR + "/" + ORGDIR + "/" + AUTHJSON)
	if err != nil {
		return nil, err
	}
	auth := &Authentication{}
	err = json.Unmarshal(data, auth)
	if err != nil {
		return nil, err
	}
	/*TODO: Go has support for other modes which do support integrity and authentication checks. As rossum said you can use GCM or CCM. You can find lots of examples on godoc.org. For example HashiCorp's memberlist library.
	// need to initialize the block cipher:
	auth.block, err :=
	*/
	return auth, nil
}

func createAuthentication(path, username, dirname string) (*Authentication, error) {
	// get new directory identifier
	id, err := newIdentifier()
	if err != nil {
		return nil, err
	}
	// Bcrypt username
	userhash, err := bcrypt.GenerateFromPassword([]byte(username), 10)
	if err != nil {
		return nil, err
	}
	// Make a new secure key:
	data := make([]byte, KEYLENGTH)
	_, err = rand.Read(data)
	if err != nil {
		return nil, err
	}
	/*TODO store key and init block! maybe make own function?*/
	return &Authentication{
		User:    string(userhash),
		Dirname: dirname,
		DirID:   id,
		Key:     ""}, nil
}

func (a *Authentication) store(root string) error {
	// write auth file
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(root+"/"+TINZENITEDIR+"/"+ORGDIR+"/"+AUTHJSON, data, FILEPERMISSIONMODE)
}
