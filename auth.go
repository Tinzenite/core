package core

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"io/ioutil"
	"log"

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
	// Bcrypt password
	passhash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		return nil, err
	}
	auth.initCipher(passhash)
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
	// Bcrypt password
	passhash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
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
	return ioutil.WriteFile(root+"/"+TINZENITEDIR+"/"+ORGDIR+"/"+AUTHJSON, data, FILEPERMISSIONMODE)
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

func TestCryptoStuff() {
	// catch errors so that I can read them :P
	defer func() {
		if r := recover(); r != nil {
			log.Println(r)
		}
	}()
	encrypted := make([]byte, 16*3)
	outA := make([]byte, 16*3)
	outB := make([]byte, 16*3)
	cblock, err := aes.NewCipher([]byte("1234567812345678"))
	if err != nil {
		log.Println("a: " + err.Error())
		return
	}
	wblock, err := aes.NewCipher([]byte("1234367812343678"))
	if err != nil {
		log.Println(err.Error())
		return
	}
	cblock.Encrypt(encrypted, []byte("Top secret, do not reveal to outsider under penalty of death!   ")[:16])
	cblock.Decrypt(outA, encrypted)
	wblock.Decrypt(outB, encrypted)
	log.Printf("Correct: %s\nWrong: %s\n", outA, outB)
}
