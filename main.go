package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

/*
Tinzenite is the struct on which all important operations should be called.
*/
type Tinzenite struct {
	Path     string
	auth     *Authentication
	selfpeer *Peer
	channel  *Channel
	allPeers []*Peer
	model    *model
}

/*
CreateTinzenite makes a directory a new Tinzenite directory. Will return error
if already so.
*/
func CreateTinzenite(dirname, dirpath, peername, username string) (*Tinzenite, error) {
	if IsTinzenite(dirpath) {
		return nil, ErrIsTinzenite
	}
	id, err := newIdentifier()
	if err != nil {
		return nil, err
	}
	// Bcrypt username
	userhash, err := bcrypt.GenerateFromPassword([]byte(username), 10)
	if err != nil {
		return nil, err
	}
	// Build
	tinzenite := &Tinzenite{
		Path: dirpath,
		auth: &Authentication{
			User:    string(userhash),
			Dirname: dirname,
			DirID:   id,
			Key:     "TODO"}}
	// build channel
	channel, err := CreateChannel(peername, nil, tinzenite)
	if err != nil {
		return nil, err
	}
	tinzenite.channel = channel
	// build self peer
	address, err := channel.Address()
	if err != nil {
		return nil, err
	}
	peerhash, err := newIdentifier()
	if err != nil {
		return nil, err
	}
	peer := &Peer{
		Name:           peername,
		Address:        address,
		Protocol:       Tox,
		Identification: peerhash}
	tinzenite.selfpeer = peer
	tinzenite.allPeers = []*Peer{peer}
	// make .tinzenite so that model can work
	err = tinzenite.makeDotTinzenite()
	if err != nil {
		return nil, err
	}
	// build model (can block for long!)
	m, err := createModel(dirpath, peer.Identification)
	if err != nil {
		RemoveTinzenite(dirpath)
		return nil, err
	}
	tinzenite.model = m
	// finally store initial copy
	err = tinzenite.Store()
	if err != nil {
		RemoveTinzenite(dirpath)
		return nil, err
	}
	// save that this directory is now a tinzenite dir
	err = tinzenite.storeGlobalConfig()
	if err != nil {
		RemoveTinzenite(dirpath)
		return nil, err
	}
	/*TODO later implement that model updates are sent to all online peers --> channel and func must be init here*/
	return tinzenite, nil
}

/*
LoadTinzenite will try to load the given directory path as a Tinzenite directory.
If not one it won't work: use CreateTinzenite to create a new peer.
*/
func LoadTinzenite(dirpath string) (*Tinzenite, error) {
	if !IsTinzenite(dirpath) {
		return nil, ErrNotTinzenite
	}
	t := &Tinzenite{Path: dirpath}
	// load auth
	auth, err := loadAuthentication(dirpath)
	if err != nil {
		return nil, err
	}
	t.auth = auth
	// load model
	model, err := loadModel(dirpath)
	if err != nil {
		return nil, err
	}
	t.model = model
	// load peer list
	peers, err := loadPeers(dirpath)
	if err != nil {
		return nil, err
	}
	t.allPeers = peers
	// load tox dump
	selfToxDump, err := loadToxDump(dirpath)
	if err != nil {
		return nil, err
	}
	t.selfpeer = selfToxDump.SelfPeer
	// prepare channel
	channel, err := CreateChannel(t.selfpeer.Name, selfToxDump.ToxData, t)
	if err != nil {
		return nil, err
	}
	t.channel = channel
	return t, nil
}

/*
RemoveTinzenite directory. Specifically leaves all user files but removes all
Tinzenite specific items.
*/
func RemoveTinzenite(path string) error {
	if !IsTinzenite(path) {
		return ErrNotTinzenite
	}
	/* TODO remove from directory list*/
	return os.RemoveAll(path + "/" + TINZENITEDIR)
}

/*
SyncModel TODO

fetches model from other peers and syncs (this is for manual sync)
*/
func (t *Tinzenite) SyncModel() error {
	// first ensure that local model is up to date
	err := t.model.Update()
	if err != nil {
		return err
	}
	// iterate over all known peers
	// TODO the following can be parallelized!
	for _, peer := range t.allPeers {
		if strings.EqualFold(peer.Address, t.selfpeer.Address) {
			continue
		}
		online, _ := t.channel.IsOnline(peer.Address)
		if !online {
			continue
		}
		log.Printf("Sending request to %s.\n", peer.Address)
		/*TODO - also make this concurrent?*/
		t.channel.Send(peer.Address, "Want update! <-- TODO replace this message")
		// if online -> continue
		// if not init -> init
		// sync
		/*TODO must store init state in allPeers too, also in future encryption*/
	}
	return nil
}

/*
Address of this Tinzenite peer.
*/
func (t *Tinzenite) Address() string {
	return t.selfpeer.Address
}

/*
Close cleanly stores everything and shuts Tinzenite down.
*/
func (t *Tinzenite) Close() {
	// store all information
	t.Store()
	// FINALLY close (afterwards because I still need info from channel for store!)
	t.channel.Close()
}

/*
Store the tinzenite directory structure to disk. Will resolve all important
objects and store them so that it can later be reloaded.
*/
func (t *Tinzenite) Store() error {
	err := t.makeDotTinzenite()
	if err != nil {
		return err
	}
	// write all peers to files
	for _, peer := range t.allPeers {
		err := peer.store(t.Path)
		if err != nil {
			return err
		}
	}
	// store local peer info with toxdata
	toxData, err := t.channel.ToxData()
	if err != nil {
		return err
	}
	toxPeerDump := &toxPeerDump{
		SelfPeer: t.selfpeer,
		ToxData:  toxData}
	err = toxPeerDump.store(t.Path)
	if err != nil {
		return err
	}
	// finally store auth file
	return t.auth.store(t.Path)
}

/*
Connect this tinzenite to another peer, beginning the bootstrap process.
*/
func (t *Tinzenite) Connect(address string) error {
	return t.channel.RequestConnection(address, t.selfpeer)
}

/*
CallbackNewConnection is called when a new connection request comes in.
*/
func (t *Tinzenite) callbackNewConnection(address, message string) {
	log.Printf("New connection from <%s> with message <%s>\n", address, message)
	err := t.channel.AcceptConnection(address)
	if err != nil {
		log.Println(err.Error())
		return
	}
	/*TODO actually this should be read from disk once the peer has synced... oO
	Correction: read from message other peer info */
	newID, _ := newIdentifier()
	t.allPeers = append(t.allPeers, &Peer{
		Identification: newID,   // must be read from message
		Name:           message, // must be read from message
		Address:        address,
		Protocol:       Tox})
	// actually we just want to get type and confidence from the user here, and if everything
	// is okay we accept the connection --> then what? need to bootstrap him...
}

/*
CallbackMessage is called when a message is received.
*/
func (t *Tinzenite) callbackMessage(address, message string) {
	switch message {
	case "model":
		t.channel.Send(address, t.model.String())
	case "auth":
		authbin, _ := json.Marshal(t.auth)
		t.channel.Send(address, string(authbin))
	case "create": // TEST ONLY
		/* TODO continue these tests!*/
		// CREATE
		os.Create(t.Path + "/test.txt")
		obj, _ := createObjectInfo(t.Path, "test.txt", "otheridhere")
		t.model.ApplyUpdateMessage(&UpdateMessage{
			Operation: Create,
			Object:    *obj})
	case "modify":
		// MODIFY
		obj, _ := createObjectInfo(t.Path, "test.txt", "otheridhere")
		orig, _ := t.model.Objinfo[t.Path+"/test.txt"]
		obj.Version[t.model.SelfID] = orig.Version[t.model.SelfID]
		// if orig already has, increase further
		value, ok := orig.Version["otheridhere"]
		if ok {
			obj.Version["otheridhere"] = value
		}
		// add one new version
		obj.Version.Increase("otheridhere")
		// write change
		ioutil.WriteFile(t.Path+"/test.txt", []byte("hello world"), FILEPERMISSIONMODE)
		t.model.ApplyUpdateMessage(&UpdateMessage{
			Operation: Modify,
			Object:    *obj})
	case "conflict":
		// MODIFY that creates merge conflict
		obj, _ := createObjectInfo(t.Path, "test.txt", "otheridhere")
		obj.Version[t.model.SelfID] = -1
		obj.Version.Increase("otheridhere") // the remote change
		log.Println("Sending: " + obj.Version.String())
		ioutil.WriteFile(t.Path+"/test.txt", []byte("hello world"), FILEPERMISSIONMODE)
		t.model.ApplyUpdateMessage(&UpdateMessage{
			Operation: Modify,
			Object:    *obj})
	case "delete":
		// DELETE
		obj, err := createObjectInfo(t.Path, "test.txt", "otheridhere")
		if err != nil {
			log.Println(err.Error())
			return
		}
		os.Remove(t.Path + "/test.txt")
		t.model.ApplyUpdateMessage(&UpdateMessage{
			Operation: Remove,
			Object:    *obj})
	default:
		t.channel.Send(address, "ACK")
	}
}

/*
storeGlobalConfig stores the path value into the user's home directory so that clients
can locate it.
*/
func (t *Tinzenite) storeGlobalConfig() error {
	// ready outside data
	user, err := user.Current()
	if err != nil {
		return err
	}
	path := user.HomeDir + "/.config/tinzenite"
	err = makeDirectory(path)
	if err != nil {
		return err
	}
	path += "/" + DIRECTORYLIST
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, FILEPERMISSIONMODE)
	if err != nil {
		return err
	}
	defer file.Close()
	// write path to file
	_, err = file.WriteString(t.Path + "\n")
	if err != nil {
		return err
	}
	// ensure that the file is valid
	return PrettifyDirectoryList()
}

/*
makeDotTinzenite creates the directory structure for the .tinzenite directory.

TODO: optimize this function to check if it even needs to create these dir first?
*/
func (t *Tinzenite) makeDotTinzenite() error {
	root := t.Path + "/" + TINZENITEDIR
	// build directory structure
	return makeDirectories(root, ORGDIR+"/"+PEERSDIR, "temp", "removed", LOCAL)
}
