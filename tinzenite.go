package core

import (
	"fmt"
	"log"
	"os"
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
	muteFlag       bool
	stop           chan bool
	wg             sync.WaitGroup
	peerValidation PeerValidation
}

/*
SyncRemote updates first locally and then sync remotely if other peers are
connected. NOTE: Both sync methods can block for a potentially long time,
especially when first run!
*/
func (t *Tinzenite) SyncRemote() error {
	// mute updates because we'll sync models later
	t.muteFlag = true
	// defer setting it back guaranteed
	defer func() { t.muteFlag = false }()
	// first ensure that local model is up to date
	err := t.SyncLocal()
	if err != nil {
		return err
	}
	online, err := t.channel.OnlineAddresses()
	if err != nil {
		return err
	}
	for _, address := range online {
		t.cInterface.SyncModel(address)
	}
	return nil
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
Address of this Tinzenite peer that can be used to connect to.
*/
func (t *Tinzenite) Address() (string, error) {
	return t.channel.ConnectionAddress()
}

/*
Name of this Tinzenite peer.
*/
func (t *Tinzenite) Name() string {
	return t.selfpeer.Name
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
	err := shared.MakeDotTinzenite(t.Path)
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
	// update model for tinzenite dir to catch above stores
	err = t.model.PartialUpdate(t.Path + "/" + shared.TINZENITEDIR)
	if err != nil {
		return err
	}
	// finally store model (last because peers and org stuff is included!)
	return t.model.Store()
}

/*
PrintStatus returns a formatted string of the peer status.
*/
func (t *Tinzenite) PrintStatus() string {
	var out string
	out += "Online:\n"
	addresses, err := t.channel.FriendAddresses()
	if err != nil {
		out += "channel.FriendAddresses failed!"
	} else {
		var count int
		for _, address := range addresses {
			online, err := t.channel.IsOnline(address)
			var insert string
			if err != nil {
				insert = "ERROR"
			} else {
				insert = fmt.Sprintf("%v", online)
			}
			out += address[:16] + " :: " + insert + "\n"
			count++
		}
		out += "Total friends: " + fmt.Sprintf("%d", count)
	}
	return out
}

/*
DisconnectPeer does exactly that. NOTE: this is a passive action and doesn't do
anything except remove the peer from the network. The peer is not further
notified.

TODO: maybe not use name but Identification?
*/
func (t *Tinzenite) DisconnectPeer(peerName string) {
	var newPeers []*shared.Peer
	for _, peer := range t.allPeers {
		if t.selfpeer.Identification == peer.Identification {
			continue
		}
		if peer.Name == peerName {
			log.Println("Removing", peer.Name, "at", peer.Address[:8])
			// delete peer file
			path := shared.CreatePath(t.Path, shared.TINZENITEDIR+"/"+shared.ORGDIR+"/"+shared.PEERSDIR+"/"+peer.Identification+shared.ENDING)
			err := t.model.ApplyRemove(path, nil)
			if err != nil {
				log.Println("DisconnectPeer:", err)
			}
			// remove from channel
			err = t.channel.RemoveConnection(peer.Address)
			if err != nil {
				log.Println("DisconnectPeer:", err)
			}
			// continue does not readd to tinzenite, removing the reference to it
			continue
		}
		newPeers = append(newPeers, peer)
	}
	t.allPeers = newPeers
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
Send the given message to the peer with the address if online.
*/
func (t *Tinzenite) send(address, msg string) error {
	return t.channel.Send(address, msg)
}

/*
Merge an update message to the local model.
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
			// log.Println("Core:", "Merge not required as updates are in sync!")
			// so all we need to do is apply the version update
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
			// if muted don't send updates
			if t.muteFlag {
				continue
			}
			online, err := t.channel.OnlineAddresses()
			if err != nil {
				log.Println("Background:", err)
			}
			for _, address := range online {
				t.channel.Send(address, msg.String())
			}
		} // select
	} // for
}
