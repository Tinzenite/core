package core

import (
	"log"
	"os"
	"os/user"
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
	model    *Model
}

/*
CreateTinzenite makes a directory a new Tinzenite directory. Will return error
if already so.
*/
func CreateTinzenite(dirname, dirpath, peername, username string) (*Tinzenite, error) {
	if IsTinzenite(dirpath) {
		return nil, ErrIsTinzenite
	}
	hash, err := newIdentifier()
	if err != nil {
		return nil, err
	}
	/*TODO Bcrypt username!*/
	// Build
	tinzenite := &Tinzenite{
		Path: dirpath,
		auth: &Authentication{
			User:    username,
			Dirname: dirname,
			DirID:   hash,
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
	// save that this directory is now a tinzenite dir
	err = tinzenite.storeGlobalConfig()
	if err != nil {
		return nil, err
	}
	// make .tinzenite so that model can work
	err = makeDirectory(dirpath + "/" + TINZENITEDIR)
	if err != nil {
		return nil, err
	}
	// build model (can block for long!)
	m, err := LoadModel(dirpath)
	if err != nil {
		return nil, err
	}
	tinzenite.model = m
	// finally store initial copy
	err = tinzenite.Store()
	if err != nil {
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
	model, err := LoadModel(dirpath)
	if err != nil {
		return nil, err
	}
	t.model = model
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
	for _, peer := range t.allPeers {
		if peer == t.selfpeer {
			continue
		}
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
	root := t.Path + "/" + TINZENITEDIR
	// build directory structure
	err := makeDirectories(root,
		ORGDIR+"/"+PEERSDIR, "temp", "removed", LOCAL)
	if err != nil {
		return err
	}
	// write all peers to files
	for _, peer := range t.allPeers {
		log.Println("Storing " + peer.Name)
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
func (t *Tinzenite) CallbackNewConnection(address, message string) {
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
func (t *Tinzenite) CallbackMessage(address, message string) {
	// log.Printf("Message from <%s> with message <%s>\n", address, message)
	switch message {
	case "model":
		t.channel.Send(address, t.model.String())
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
