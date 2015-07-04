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
	block   cipher.Block /*TODO use authenticated encryption*/
}

/*
loadAuthentication loads the auth.json file for the given Tinzenite directory.
*/
func loadAuthentication(path string, password string) (*Authentication, error) {
	data, err := ioutil.ReadFile(path + "/" + TINZENITEDIR + "/" + ORGDIR + "/" + AUTHJSON)
	if err != nil {
		return nil, err
	}
	auth := &Authentication{}
	err = json.Unmarshal(data, auth)
	if err != nil {
		return nil, err
	}
	/*TODO check if password ok and use to decrypt key for init*/
	auth.initCipher(password)
	return auth, nil
}

func createAuthentication(path, dirname, username, password string) (*Authentication, error) {
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
	/*TODO store key encrypted with password!*/
	auth := &Authentication{
		User:    string(userhash),
		Dirname: dirname,
		DirID:   id,
		Key:     password}
	// init cipher once key is available
	auth.initCipher(password)
	return auth, nil
}

/*
Store the authentication file to disk as json.
*/
func (a *Authentication) Store(root string) error {
	// write auth file
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(root+"/"+TINZENITEDIR+"/"+ORGDIR+"/"+AUTHJSON, data, FILEPERMISSIONMODE)
}

func (a *Authentication) initCipher(password string) {
	/*TODO use password to get key, don't need cipher if that fails*/
	/*TODO: Go has support for other modes which do support integrity and authentication checks. As rossum said you can use GCM or CCM. You can find lots of examples on godoc.org. For example HashiCorp's memberlist library.
	// need to initialize the block cipher:
	auth.block, err :=
	*/
	/*TODO implement what happens when password fails! --> strong error!*/
}
