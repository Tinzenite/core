package core

import (
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"strings"
	"sync"

	"github.com/tinzenite/channel"
	"github.com/tinzenite/model"
	"github.com/tinzenite/shared"
)

/*
Tinzenite is the struct on which all important operations should be called.
*/
type Tinzenite struct {
	Path           string
	auth           *Authentication
	selfpeer       *shared.Peer
	channel        *channel.Channel
	cInterface     *chaninterface
	allPeers       []*shared.Peer
	model          *model.Model
	sendChannel    chan shared.UpdateMessage
	stop           chan bool
	wg             sync.WaitGroup
	peerValidation PeerValidation
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
	// update peers
	err := t.applyPeers()
	if err != nil {
		return err
	}
	// update model
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
	msg := shared.CreateRequestMessage(shared.ReModel, "TODO")
	t.sendAll(msg.String())
	/*TODO implement model detection on receive? Not here, I guess?*/
	return nil
}

/*
Address of this Tinzenite peer that can be used to connect to.
*/
func (t *Tinzenite) Address() (string, error) {
	return t.channel.ConnectionAddress()
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
		err := peer.Store(t.Path)
		if err != nil {
			return err
		}
	}
	// store local peer info with toxdata
	toxData, err := t.channel.ToxData()
	if err != nil {
		return err
	}
	toxPeerDump := &shared.ToxPeerDump{
		SelfPeer: t.selfpeer,
		ToxData:  toxData}
	err = toxPeerDump.Store(t.Path)
	if err != nil {
		return err
	}
	// store auth file
	err = t.auth.Store(t.Path)
	if err != nil {
		return err
	}
	// store bootstrap
	err = t.cInterface.Store(t.Path)
	if err != nil {
		return err
	}
	// update model for tinzenite dir to catch above stores
	err = t.model.PartialUpdate(t.Path + "/" + shared.TINZENITEDIR)
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
	return t.cInterface.Connect(address)
}

/*
applyPeers loads all peers and readies the communication channel accordingly.
*/
func (t *Tinzenite) applyPeers() error {
	peers, err := shared.LoadPeers(t.Path)
	if err != nil {
		return err
	}
	// make sure they are all tox ready
	for _, peer := range peers {
		// tox will return an error if the address has already been added, so we just ignore it
		_ = t.channel.AcceptConnection(peer.Address)
	}
	// finally apply
	t.allPeers = peers
	return nil
}

/*
Send the given message string to all online and connected peers.
*/
func (t *Tinzenite) sendAll(msg string) {
	for _, peer := range t.allPeers {
		if strings.EqualFold(peer.Address, t.selfpeer.Address) {
			continue
		}
		online, err := t.channel.IsOnline(peer.Address)
		if err != nil {
			log.Println(err)
			continue
		}
		if online {
			/*TODO - also make this concurrent?*/
			err := t.channel.Send(peer.Address, msg)
			if err != nil {
				log.Println(err.Error(), peer.Address)
			}
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
func (t *Tinzenite) merge(msg *shared.UpdateMessage) error {
	relPath := shared.CreatePath(t.Path, msg.Object.Path)
	// first: apply local changes to model (this is why writing PartialUpdate was no waste of time, isn't this cool?! :D)
	err := t.model.PartialUpdate(relPath.FullPath())
	if err != nil {
		return err
	}
	// check if content is same, no need for merge then (except for version)
	stin, err := t.model.GetInfo(relPath)
	if err != nil {
		log.Println("Core:", "Can not check if content is same!")
	} else {
		if stin.Content == msg.Object.Content {
			log.Println("Core:", "Merge not required as updates are in sync!")
			// so all we need to do is apply the version update
			/*TODO: we need applymodify WITHOUT the fetching of the file...*/
			t.model.ApplyModify(relPath, &msg.Object)
			return nil
		}
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
	err = t.model.ApplyRemove(relPath, nil)
	if err != nil {
		return err
	}
	// fourth: change path and apply remote as create
	msg.Operation = shared.OpCreate
	msg.Object.Path = relPath.SubPath() + REMOTE
	msg.Object.Name = relPath.LastElement() + REMOTE
	/*TODO what of the id? For now to be sure: new one.*/
	oldID := msg.Object.Identification
	msg.Object.Identification, err = shared.NewIdentifier()
	if err != nil {
		return err
	}
	// new id --> rename temp file
	tempPath := t.Path + "/" + shared.TINZENITEDIR + "/" + shared.TEMPDIR
	err = os.Rename(tempPath+"/"+oldID, tempPath+"/"+msg.Object.Identification)
	if err != nil {
		log.Println("Updating remote object file failed!")
		return err
	}
	// fifth: create remote file
	err = t.model.ApplyCreate(relPath.Apply(relPath.FullPath()+REMOTE), &msg.Object)
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
	err = shared.MakeDirectory(path)
	if err != nil {
		return err
	}
	path += "/" + shared.DIRECTORYLIST
	file, err := os.OpenFile(path, shared.FILEFLAGCREATEAPPEND, shared.FILEPERMISSIONMODE)
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
	return shared.PrettifyDirectoryList()
}

/*
makeDotTinzenite creates the directory structure for the .tinzenite directory
including the .tinignore file required for it.
*/
func (t *Tinzenite) makeDotTinzenite() error {
	root := t.Path + "/" + shared.TINZENITEDIR
	// build directory structure
	err := shared.MakeDirectories(root, shared.ORGDIR+"/"+shared.PEERSDIR, shared.TEMPDIR, shared.REMOVEDIR, shared.LOCALDIR, shared.RECEIVINGDIR)
	if err != nil {
		return err
	}
	// write required .tinignore file
	return ioutil.WriteFile(root+"/"+shared.TINIGNORE, []byte(TINDIRIGNORE), shared.FILEPERMISSIONMODE)
}

/*
initialize the background process.
*/
func (t *Tinzenite) initialize() {
	// prepare send channel that will distribute updates
	t.wg.Add(1)
	t.stop = make(chan bool, 1)
	t.sendChannel = make(chan shared.UpdateMessage, 1)
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
