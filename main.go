package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strings"
	"sync"
)

/*
Tinzenite is the struct on which all important operations should be called.
*/
type Tinzenite struct {
	Path        string
	auth        *Authentication
	selfpeer    *Peer
	channel     *Channel
	allPeers    []*Peer
	model       *model
	sendChannel chan UpdateMessage
	stop        chan bool
	wg          sync.WaitGroup
}

/*
CreateTinzenite makes a directory a new Tinzenite directory. Will return error
if already so.
*/
func CreateTinzenite(dirname, dirpath, peername, username, password string) (*Tinzenite, error) {
	if IsTinzenite(dirpath) {
		return nil, ErrIsTinzenite
	}
	// get auth data
	auth, err := createAuthentication(dirpath, dirname, username, password)
	if err != nil {
		return nil, err
	}
	// Build
	tinzenite := &Tinzenite{
		Path: dirpath,
		auth: auth}
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
		Protocol:       CmTox,
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
	// store initial copy
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
	tinzenite.initialize()
	return tinzenite, nil
}

/*
LoadTinzenite will try to load the given directory path as a Tinzenite directory.
If not one it won't work: use CreateTinzenite to create a new peer.
*/
func LoadTinzenite(dirpath, password string) (*Tinzenite, error) {
	if !IsTinzenite(dirpath) {
		return nil, ErrNotTinzenite
	}
	t := &Tinzenite{Path: dirpath}
	// load auth
	auth, err := loadAuthentication(dirpath, password)
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
	t.initialize()
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
Sync the entire file system model of the directory, first locally and then
remotely if other peers are connected. NOTE: All Sync{|Local|Remote} methods can
block for a potentially long time, especially when first run!
*/
func (t *Tinzenite) Sync() error {
	// first ensure that local model is up to date
	err := t.SyncLocal()
	if err != nil {
		return err
	}
	return t.SyncRemote()
}

/*
SyncLocal changes. Will send updates to connected peers but not synchronize with
other peers.
*/
func (t *Tinzenite) SyncLocal() error {
	return t.model.Update()
}

/*
SyncRemote changes. Will request the models of connected peers and merge them
to the local peer.

TODO: fetches model from other peers and syncs (this is for manual sync)
*/
func (t *Tinzenite) SyncRemote() error {
	// iterate over all known peers
	// TODO the following can be parallelized!
	t.send("Want update! <-- TODO replace with something sensible...")
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
	// send stop signal
	t.stop <- false
	// wait for it to close
	t.wg.Wait()
	// store all information
	t.Store()
	// FINALLY close (afterwards because I still need info from channel for store!)
	t.channel.Close()
}

/*
Store the tinzenite directory structure to disk. Will resolve all important
objects and store them so that it can later be reloaded. NOTE: Will not update
the full model, so be sure to have called Update() to guarantee an up to date
save.
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
	// store auth file
	err = t.auth.Store(t.Path)
	if err != nil {
		return err
	}
	// update model for tinzenite dir to catch above stores
	err = t.model.PartialUpdate(t.Path + "/" + TINZENITEDIR)
	if err != nil {
		return err
	}
	// finally store model (last because peers and org stuff is included!)
	return t.model.Store()
}

/*
Connect this tinzenite to another peer, beginning the bootstrap process.
*/
func (t *Tinzenite) Connect(address string) error {
	return t.channel.RequestConnection(address, t.selfpeer)
}

/*
Send the given message string to all online and connected peers.
*/
func (t *Tinzenite) send(msg string) {
	for _, peer := range t.allPeers {
		if strings.EqualFold(peer.Address, t.selfpeer.Address) {
			continue
		}
		online, _ := t.channel.IsOnline(peer.Address)
		if !online {
			continue
		}
		/*TODO - also make this concurrent?*/
		t.channel.Send(peer.Address, msg)
		// if online -> continue
		// if not init -> init
		// sync
		/*TODO must store init state in allPeers too, also in future encryption*/
	}
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
		Protocol:       CmTox})
	// actually we just want to get type and confidence from the user here, and if everything
	// is okay we accept the connection --> then what? need to bootstrap him...
}

/*
CallbackMessage is called when a message is received.
*/
func (t *Tinzenite) callbackMessage(address, message string) {
	// find out type of message
	v := &Message{}
	err := json.Unmarshal([]byte(message), v)
	if err == nil {
		switch msgType := v.Type; msgType {
		case MsgUpdate:
			msg := &UpdateMessage{}
			err := json.Unmarshal([]byte(message), msg)
			if err != nil {
				log.Println(err.Error())
				return
			}
			reqMsg := createRequestMessage(ReObject, msg.Object.Identification)
			t.channel.Send(address, reqMsg.String())
			/* TODO implement application of msg as wit manual command but will need to fetch file first...*/
		case MsgRequest:
			log.Println("Request received!")
			t.channel.Send(address, "Sending File (TODO)")
			/* TODO implement application of msg as wit manual command but will need to fetch file first...*/
		default:
			log.Printf("Unknown object sent: %s!\n", msgType)
		}
		// If it was JSON, we're done if we can't do anything with it
		return
	}
	// if unmarshal didn't work check for plain commands:
	switch message {
	case "model":
		t.channel.Send(address, t.model.String())
	case "auth":
		authbin, _ := json.Marshal(t.auth)
		t.channel.Send(address, string(authbin))
	case "create":
		// CREATE
		// messy but works: create file correctly, create objs, then move it to the correct temp location
		// first named create.txt to enable testing of create merge
		os.Create(t.Path + "/create.txt")
		ioutil.WriteFile(t.Path+"/create.txt", []byte("bonjour!"), FILEPERMISSIONMODE)
		obj, _ := createObjectInfo(t.Path, "create.txt", "otheridhere")
		os.Rename(t.Path+"/create.txt", t.Path+"/"+TINZENITEDIR+"/"+TEMPDIR+"/"+obj.Identification)
		obj.Name = "test.txt"
		obj.Path = "test.txt"
		msg := &UpdateMessage{
			Operation: OpCreate,
			Object:    *obj}
		err := t.model.ApplyUpdateMessage(msg)
		if err == errConflict {
			err := t.merge(msg)
			if err != nil {
				log.Println("Merge: " + err.Error())
			}
		} else if err != nil {
			log.Println(err.Error())
		}
	case "modify":
		// MODIFY
		obj, _ := createObjectInfo(t.Path, "test.txt", "otheridhere")
		orig, _ := t.model.Objinfo[t.Path+"/test.txt"]
		// id must be same
		obj.Identification = orig.Identification
		// version apply so that we can always "update" it
		obj.Version[t.model.SelfID] = orig.Version[t.model.SelfID]
		// if orig already has, increase further
		value, ok := orig.Version["otheridhere"]
		if ok {
			obj.Version["otheridhere"] = value
		}
		// add one new version
		obj.Version.Increase("otheridhere")
		err := t.model.ApplyUpdateMessage(&UpdateMessage{
			Operation: OpModify,
			Object:    *obj})
		if err != nil {
			log.Println(err.Error())
		}
	case "sendmodify":
		path := t.Path + "/" + TINZENITEDIR + "/" + TEMPDIR
		orig, _ := t.model.Objinfo[t.Path+"/test.txt"]
		// write change to file in temp, simulating successful download
		ioutil.WriteFile(path+"/"+orig.Identification, []byte("send modify hello world!"), FILEPERMISSIONMODE)
	case "testdir":
		// Test creation and removal of directory
		makeDirectory(t.Path + "/dirtest")
		obj, _ := createObjectInfo(t.Path, "dirtest", "dirtestpeer")
		os.Remove(t.Path + "/dirtest")
		err := t.model.ApplyUpdateMessage(&UpdateMessage{
			Operation: OpCreate,
			Object:    *obj})
		if err != nil {
			log.Println(err.Error())
		}
	case "conflict":
		// MODIFY that creates merge conflict
		path := t.Path + "/" + TINZENITEDIR + "/" + TEMPDIR
		ioutil.WriteFile(t.Path+"/merge.txt", []byte("written from conflict test"), FILEPERMISSIONMODE)
		obj, _ := createObjectInfo(t.Path, "merge.txt", "otheridhere")
		os.Rename(t.Path+"/merge.txt", path+"/"+obj.Identification)
		obj.Path = "test.txt"
		obj.Name = "test.txt"
		obj.Version[t.model.SelfID] = -1
		obj.Version.Increase("otheridhere") // the remote change
		msg := &UpdateMessage{
			Operation: OpModify,
			Object:    *obj}
		err := t.model.ApplyUpdateMessage(msg)
		if err == errConflict {
			err := t.merge(msg)
			if err != nil {
				log.Println("Merge: " + err.Error())
			}
		} else {
			log.Println("WHY NO MERGE?!")
		}
	case "delete":
		// DELETE
		obj, err := createObjectInfo(t.Path, "test.txt", "otheridhere")
		if err != nil {
			log.Println(err.Error())
			return
		}
		os.Remove(t.Path + "/test.txt")
		t.model.ApplyUpdateMessage(&UpdateMessage{
			Operation: OpRemove,
			Object:    *obj})
		/*TODO implement remove merge conflict!*/
	default:
		t.channel.Send(address, "ACK")
	}
}

/*
Merge an update message to the local model.

TODO: with move implemented one of the merged file copies can be kept and simply
renamed, also solving the ID problem. Look into this!
*/
func (t *Tinzenite) merge(msg *UpdateMessage) error {
	relPath := createPath(t.Path, msg.Object.Path)
	// first: apply local changes to model (this is why writing PartialUpdate was no waste of time, isn't this cool?! :D)
	err := t.model.PartialUpdate(relPath.FullPath())
	if err != nil {
		return err
	}
	// second: move to new name
	err = os.Rename(relPath.FullPath(), relPath.FullPath()+".LOCAL")
	if err != nil {
		log.Println("Original can not be found!")
		return err
	}
	err = t.model.PartialUpdate(relPath.FullPath() + ".LOCAL")
	if err != nil {
		return err
	}
	// third: remove original
	err = t.model.applyRemove(relPath, nil)
	if err != nil {
		return err
	}
	// fourth: change path and apply remote as create
	msg.Operation = OpCreate
	msg.Object.Path = relPath.Subpath() + ".REMOTE"
	msg.Object.Name = relPath.LastElement() + ".REMOTE"
	/*TODO what of the id? For now to be sure: new one.*/
	oldID := msg.Object.Identification
	msg.Object.Identification, err = newIdentifier()
	if err != nil {
		return err
	}
	// new id --> rename temp file
	tempPath := t.Path + "/" + TINZENITEDIR + "/" + TEMPDIR
	err = os.Rename(tempPath+"/"+oldID, tempPath+"/"+msg.Object.Identification)
	if err != nil {
		log.Println("Updating remote object file failed!")
		return err
	}
	// fifth: create remote file
	err = t.model.applyCreate(relPath.Apply(relPath.FullPath()+".REMOTE"), &msg.Object)
	if err != nil {
		return err
	}
	return nil
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
makeDotTinzenite creates the directory structure for the .tinzenite directory
including the .tinignore file required for it.
*/
func (t *Tinzenite) makeDotTinzenite() error {
	root := t.Path + "/" + TINZENITEDIR
	// build directory structure
	err := makeDirectories(root, ORGDIR+"/"+PEERSDIR, TEMPDIR, REMOVEDIR, LOCALDIR)
	if err != nil {
		return err
	}
	// write required .tinignore file
	return ioutil.WriteFile(root+"/"+TINIGNORE, []byte(TINDIRIGNORE), FILEPERMISSIONMODE)
}

/*
initialize the background process.
*/
func (t *Tinzenite) initialize() {
	// prepare send channel that will distribute updates
	t.wg.Add(1)
	t.stop = make(chan bool, 1)
	t.sendChannel = make(chan UpdateMessage, 1)
	go t.background()
	t.model.Register(t.sendChannel)
}

/*
background function that handles all async stuff that needs to be done.
*/
func (t *Tinzenite) background() {
	for {
		select {
		case <-t.stop:
			t.wg.Done()
			return
		case msg := <-t.sendChannel:
			t.send(msg.String())
		} // select
	} // for
}
