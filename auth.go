package core

import (
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
	User         string // hash of username
	Dirname      string // official name of directory
	DirID        string // random id of directory
	PasswordHash []byte // hash to check password against
	Secure       []byte // box encrypted private and public keys with password
	private      []byte // private key if unlocked
	public       []byte // public key if unlocked
}

/*
loadAuthentication loads the auth.json file for the given Tinzenite directory.
*/
func loadAuthenticationFrom(path string, password string) (*Authentication, error) {
	path = path + "/" + shared.AUTHJSON
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	auth := &Authentication{}
	err = json.Unmarshal(data, auth)
	if err != nil {
		return nil, err
	}
	// Bcrypt check password
	err = bcrypt.CompareHashAndPassword(auth.PasswordHash, []byte(password))
	if err != nil {
		// doesn't match!
		return nil, err
	}
	// IF the password was valid we use it to init the cipher
	// TODO
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
	// build authentication object
	auth := &Authentication{
		User:         string(userhash),
		Dirname:      dirname,
		DirID:        id,
		PasswordHash: passhash}
	// use password to build keys for encryption
	// TODO
	return auth, nil
}

/*
StoreTo the given path the authentication file to disk as json.
*/
func (a *Authentication) StoreTo(path string) error {
	// write auth file
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	path = path + "/" + shared.AUTHJSON
	return ioutil.WriteFile(path, data, shared.FILEPERMISSIONMODE)
}

/*
BuildChallenge builds a challenge to issue to an online peer to check whether it
is a valid trusted peer.
*/
func (a *Authentication) BuildChallenge() (string, error) {
	// TODO implement
	/*HOWTO: send number. Correct response is (number+1)*/
	//TODO: need to build message, encrypt it, and return it
	// NOTE: it looks like what we'll need to send is: encrypted and nonce, so remove nonce from WITHIN the message
	return "", shared.ErrUnsupported
}
