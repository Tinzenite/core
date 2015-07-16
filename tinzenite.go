package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strings"
	"sync"

	"github.com/tinzenite/channel"
)

/*
Tinzenite is the struct on which all important operations should be called.
*/
type Tinzenite struct {
	Path        string
	auth        *Authentication
	selfpeer    *Peer
	channel     *channel.Channel
	cInterface  *chaninterface
	allPeers    []*Peer
	model       *model
	sendChannel chan UpdateMessage
	stop        chan bool
	wg          sync.WaitGroup
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
	//the following can be parallelized!
	msg := createRequestMessage(ReModel, "")
	t.sendAll(msg.String())
	/*TODO implement model detection on receive? Not here, I guess?*/
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
	msg, err := json.Marshal(t.selfpeer)
	if err != nil {
		return err
	}
	return t.channel.RequestConnection(address, string(msg))
}

/*
Send the given message string to all online and connected peers.
*/
func (t *Tinzenite) sendAll(msg string) {
	for _, peer := range t.allPeers {
		if strings.EqualFold(peer.Address, t.selfpeer.Address) {
			continue
		}
		/*TODO - also make this concurrent?*/
		err := t.channel.Send(peer.Address, msg)
		if err != nil {
			log.Println(err.Error())
		}
		// if online -> continue
		// if not init -> init
		// sync
		/*TODO must store init state in allPeers too, also in future encryption*/
	}
}

/*
Send the given message to the peer with the address if online.
*/
func (t *Tinzenite) send(address, msg string) error {
	return t.channel.Send(address, msg)
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
	err = os.Rename(relPath.FullPath(), relPath.FullPath()+LOCAL)
	if err != nil {
		log.Println("Original can not be found!")
		return err
	}
	err = t.model.PartialUpdate(relPath.FullPath() + LOCAL)
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
	msg.Object.Path = relPath.Subpath() + REMOTE
	msg.Object.Name = relPath.LastElement() + REMOTE
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
	err = t.model.applyCreate(relPath.Apply(relPath.FullPath()+REMOTE), &msg.Object)
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
	err := makeDirectories(root, ORGDIR+"/"+PEERSDIR, TEMPDIR, REMOVEDIR, LOCALDIR, RECEIVINGDIR)
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
			t.sendAll(msg.String())
		} // select
	} // for
}
