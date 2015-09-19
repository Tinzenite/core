package core

import (
	rand "crypto/rand"
	"encoding/json"
	"errors"
	"hash/fnv"
	"io/ioutil"
	unsecure "math/rand"

	"github.com/tinzenite/shared"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/nacl/box"
)

/*
Authentication file.
*/
type Authentication struct {
	User         string    // hash of username
	Dirname      string    // official name of directory
	DirID        string    // random id of directory
	PasswordHash []byte    // hash to check password against
	Secure       []byte    // box encrypted private and public keys with password
	Nonce        *[24]byte // nonce for Secure
	private      *[32]byte // private key if unlocked
	public       *[32]byte // public key if unlocked
}

type staticRandom struct {
	random *unsecure.Rand
}

func (s staticRandom) Read(data []byte) (int, error) {
	for index := range data[:] {
		data[index] = byte(s.random.Int63())
	}
	return len(data), nil
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

func (a *Authentication) loadCrypto(password string) error {
	// ensure all values are valid
	if a.Secure == nil || a.Nonce == nil {
		return shared.ErrIllegalParameters
	}
	// get keys from password
	lockPub, lockPriv, err := a.convertPassword(password)
	if err != nil {
		return err
	}
	// unlock enc keys
	var data []byte
	data, ok := box.Open(data, a.Secure, a.Nonce, lockPub, lockPriv)
	// TODO I think this means the keys aren't ok, which means wrong password:
	if !ok {
		return errors.New("decryption failed")
	}
	// check if data is as expected
	if len(data) != 64 {
		return errors.New("secure container does not encrypt to valid keys")
	}
	// prepare keys
	a.public = new([32]byte)
	a.private = new([32]byte)
	// split enc keys from data
	for i := 0; i < 32; i++ { // first read public key from it
		a.public[i] = data[i]
	}
	for i := 0; i < 32; i++ { // then read private key from it
		a.private[i] = data[i+32]
	}
	// and done... theoretically
	return nil
}

func (a *Authentication) createCrypto(password string) error {
	// build TRULY random enc keys
	encPubKey, encPrivKey, err := box.GenerateKey(rand.Reader)
	// set them (this also immediately unlocks this auth, so no need to call load afterwards)
	a.private = encPrivKey
	a.public = encPubKey
	// build encrypted key box
	message := make([]byte, 64)
	for i := 0; i < 32; i++ { // first write public key to it
		message[i] = encPubKey[i]
	}
	for i := 0; i < 32; i++ { // then write private key to it
		message[i+32] = encPrivKey[i]
	}
	// create nonce
	nonce := make([]byte, 24)
	rand.Read(nonce)
	a.Nonce = new([24]byte)
	for i := 0; i < 24; i++ {
		a.Nonce[i] = nonce[i]
	}
	// get keys from password
	lockPub, lockPriv, err := a.convertPassword(password)
	if err != nil {
		return err
	}
	// encrypt enc keys with pub and priv
	a.Secure = box.Seal(a.Secure, message, a.Nonce, lockPub, lockPriv)
	return nil
}

/*
convertPassword generates a public and private key from the given password.
*/
func (a *Authentication) convertPassword(password string) (public *[32]byte, private *[32]byte, err error) {
	// build seed from password
	hasher := fnv.New64a()
	hasher.Write([]byte(password))
	seed := int64(hasher.Sum64())
	// use hash as seed for random
	seededRandom := unsecure.New(unsecure.NewSource(seed))
	// make seededRandom implement io.Reader interface so we can use it for box
	wrapper := staticRandom{random: seededRandom}
	// use static random to generate pub and priv keys
	public, private, err = box.GenerateKey(wrapper)
	if err != nil {
		return nil, nil, err
	}
	return public, private, nil
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
