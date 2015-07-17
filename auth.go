package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"io/ioutil"

	"github.com/tinzenite/shared"

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
	data, err := ioutil.ReadFile(path + "/" + shared.TINZENITEDIR + "/" + shared.ORGDIR + "/" + shared.AUTHJSON)
	if err != nil {
		return nil, err
	}
	auth := &Authentication{}
	err = json.Unmarshal(data, auth)
	if err != nil {
		return nil, err
	}
	/*TODO check if password ok and use to decrypt key for init*/
	// Bcrypt check password
	err = bcrypt.CompareHashAndPassword([]byte(auth.Key), []byte(password))
	if err != nil {
		// doesn't match!
		return nil, err
	}
	auth.initCipher([]byte(auth.Key))
	return auth, nil
}

func createAuthentication(path, dirname, username, password string) (*Authentication, error) {
	// get new directory identifier
	id, err := shared.NewIdentifier()
	if err != nil {
		return nil, err
	}
	// Bcrypt username
	userhash, err := bcrypt.GenerateFromPassword([]byte(username), 10)
	if err != nil {
		return nil, err
	}
	// Bcrypt password
	passhash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return nil, err
	}
	// Make a new secure key:
	data := make([]byte, shared.KEYLENGTH)
	_, err = rand.Read(data)
	if err != nil {
		return nil, err
	}
	/*TODO store key encrypted with password!*/
	auth := &Authentication{
		User:    string(userhash),
		Dirname: dirname,
		DirID:   id,
		Key:     string(passhash)}
	// init cipher once key is available
	auth.initCipher(passhash)
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
	return ioutil.WriteFile(root+"/"+shared.TINZENITEDIR+"/"+shared.ORGDIR+"/"+shared.AUTHJSON, data, shared.FILEPERMISSIONMODE)
}

func (a *Authentication) initCipher(password []byte) error {
	/*TODO use password to get key, don't need cipher if that fails*/
	/*TODO: Go has support for other modes which do support integrity and authentication checks. As rossum said you can use GCM or CCM. You can find lots of examples on godoc.org. For example HashiCorp's memberlist library. */
	// need to initialize the block cipher:
	block, err := aes.NewCipher(password)
	if err != nil {
		/*TODO implement what happens when password / cipher fails! --> strong error!*/
		return err
	}
	a.block = block
	return nil
}
