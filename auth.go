package core

import (
	"bytes"
	rand "crypto/rand"
	"encoding/binary"
	"encoding/json"
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
	User    string    // hash of username
	Dirname string    // official name of directory
	DirID   string    // random id of directory
	Secure  []byte    // box encrypted private and public keys with password
	Nonce   *[24]byte // nonce for Secure
	private *[32]byte // private key if unlocked
	public  *[32]byte // public key if unlocked
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
	// use the password to init the cipher
	err = auth.loadCrypto(password)
	if err != nil {
		return nil, err
	}
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
	// build authentication object
	auth := &Authentication{
		User:    string(userhash),
		Dirname: dirname,
		DirID:   id}
	// use password to build keys for encryption
	err = auth.createCrypto(password)
	if err != nil {
		return nil, err
	}
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
Encrypt returns the data in encrypted form, given that the keys are valid.
*/
func (a *Authentication) Encrypt(data []byte) ([]byte, error) {
	if a.private == nil || a.public == nil {
		return nil, errAuthInvalidKeys
	}
	// byte array to write encrypted data to
	var encrypted []byte
	complete := make([]byte, 24)
	// build a nonce
	nonce := a.createNonce()
	// write it to the start of encrypted
	for i, value := range nonce {
		complete[i] = value
	}
	// encrypt
	encrypted = box.Seal(encrypted, data, nonce, a.public, a.private)
	// append encrypted to complete
	complete = append(complete, encrypted...)
	return complete, nil
}

/*
Decrypt returns the unencrypted data, given that the keys are valid.
*/
func (a *Authentication) Decrypt(encrypted []byte) ([]byte, error) {
	if a.private == nil || a.public == nil {
		return nil, errAuthInvalidKeys
	}
	// ensure that a nonce is included
	if len(encrypted) <= 24 {
		return nil, errAuthMissingNonce
	}
	// read nonce
	nonce := new([24]byte)
	for i := range nonce {
		nonce[i] = encrypted[i]
	}
	// byte array to write data to
	var data []byte
	// note: encrypted is only read from the nonce onwards
	data, ok := box.Open(data, encrypted[24:], nonce, a.public, a.private)
	if !ok {
		return nil, errAuthDecryption
	}
	return data, nil
}

/*
BuildAuthentication takes the given number and returns the valid
AuthenticationMessage to send to the other side.
*/
func (a *Authentication) BuildAuthentication(number int64) (*shared.AuthenticationMessage, error) {
	// convert to data payload
	data := make([]byte, binary.MaxVarintLen64)
	_ = binary.PutVarint(data, number)
	// encrypt number with nonce
	encrypted, err := a.Encrypt(data)
	if err != nil {
		return nil, err
	}
	// write encrypted and nonce to message
	msg := shared.CreateAuthenticationMessage(encrypted)
	return &msg, nil
}

/*
ReadAuthentication takes an AuthenticationMessage and decrypts it to return the
contained number.
*/
func (a *Authentication) ReadAuthentication(msg *shared.AuthenticationMessage) (int64, error) {
	data, err := a.Decrypt(msg.Encrypted)
	if err != nil {
		return 0, err
	}
	response, err := binary.ReadVarint(bytes.NewBuffer(data[:]))
	if err != nil {
		return 0, err
	}
	return response, nil
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
	// this means the password was wrong in our case
	if !ok {
		return errAuthInvalidPassword
	}
	// check if data is as expected
	if len(data) != 64 {
		return errAuthInvalidSecure
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
	a.Nonce = a.createNonce()
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
createNonce returns a new truly random nonce fit for all purposes.
*/
func (a *Authentication) createNonce() *[24]byte {
	randValues := make([]byte, 24)
	rand.Read(randValues)
	nonce := new([24]byte)
	for i := 0; i < 24; i++ {
		nonce[i] = randValues[i]
	}
	return nonce
}
