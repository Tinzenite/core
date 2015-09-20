package core

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/big"
	"os"
	"sync"
	"time"

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
	peers          map[string]*shared.Peer
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
	// FIXME should iterate over peers instead
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
	for _, peer := range t.peers {
		err := peer.StoreTo(t.Path + "/" + shared.STOREPEERDIR)
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
	err = toxPeerDump.StoreTo(t.Path + "/" + shared.STORETOXDUMPDIR)
	if err != nil {
		return err
	}
	// store auth file
	err = t.auth.StoreTo(t.Path + "/" + shared.STOREAUTHDIR)
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
			online, err := t.channel.IsAddressOnline(address)
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
TODO: when will other peers remove it? They need to remove the contact info from the channel... FIXME
*/
func (t *Tinzenite) DisconnectPeer(peerName string) {
	newPeers := make(map[string]*shared.Peer)
	for _, peer := range t.peers {
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
			// write peer to all removals so that no removals will be orphaned
			removePath := t.Path + "/" + shared.TINZENITEDIR + "/" + shared.REMOVEDIR
			allRemovals, _ := ioutil.ReadDir(removePath)
			// for every object that is currently being removed
			for _, stat := range allRemovals {
				// write the to be removed peer as done
				err := t.model.UpdateRemovalDir(stat.Name(), peer.Identification)
				if err != nil {
					// warn if it failed
					log.Println("Tinzenite: failed to purge removed peer from removal!")
				}
			}
			// continue does not readd to tinzenite, removing the reference to it
			continue
		}
		newPeers[peer.Address] = peer
	}
	t.peers = newPeers
}

/*
applyPeers loads all peers and readies the communication channel accordingly.
*/
func (t *Tinzenite) applyPeers() error {
	peers, err := shared.LoadPeers(t.Path)
	if err != nil {
		return err
	}
	// apply to object
	t.peers = peers
	// make sure they are all tox ready
	for peerAddress, peer := range peers {
		// ignore self peer
		if peerAddress == t.selfpeer.Address {
			continue
		}
		// tox will return an error if the address has already been added, so we just ignore it
		_ = t.channel.AcceptConnection(peerAddress)
		// if not online no need to continue
		if online, _ := t.channel.IsAddressOnline(peerAddress); !online {
			continue
		}
		// if encrypted don't even try auth
		if !peer.Trusted {
			log.Println("DEBUG: not challenging untrusted peer!")
			continue
		}
		if peer.IsAuthenticated() {
			log.Println("DEBUG: not challenging already authenticated peer!")
			continue
		}
		// if peer challenge has already been issued we don't send a new one
		if number, exists := t.cInterface.challenges[peerAddress]; exists {
			// TODO retry after longish timeout
			log.Println("DEBUG: challenge already sent, won't resend!", number)
			continue
		}
		// TODO building and reading challenge can be methodized, look into that!
		// otherwise build challenge
		bigNumber, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64-1))
		if err != nil {
			log.Println("Tinzenite: failed to create challenge:", err)
			// retry later on
			continue
		}
		// convert back to int64
		number := bigNumber.Int64()
		// convert to data payload
		data := make([]byte, binary.MaxVarintLen64)
		_ = binary.PutVarint(data, number)
		// get a nonce
		nonce := t.auth.createNonce()
		// encrypt number with nonce
		encrypted, err := t.auth.Encrypt(data, nonce)
		if err != nil {
			log.Println("Tinzenite: failed to encrypt:", err)
			// retry later on
			continue
		}
		challenge := shared.CreateAuthenticationMessage(encrypted, nonce)
		// remember the challenge we sent
		t.cInterface.challenges[peerAddress] = number
		// send reply
		_ = t.channel.Send(peerAddress, challenge.JSON())
	}
	return nil
}

/*
Send the given message to the peer with the address if online. Wraps the full
featured channel.Send to ensure that no messages leak to unauthenticated peers.
*/
func (t *Tinzenite) send(address, msg string) error {
	// TODO check if authenticated and ENFORCE! FIXME
	// for now just warn if sending a secure message to unauthenticated peer
	peer, exists := t.peers[address]
	if exists {
		if !peer.IsAuthenticated() {
			log.Println("DEBUG: MESSAGE TO UNSECURE PEER!")
			// TODO for now just silently ignore
			return nil
		}
	} else {
		// not sure when this would happen, but warn nonetheless
		log.Println("DEBUG: warning, not a peer!")
	}
	return t.channel.Send(address, msg)
}

// TODO also do above for sendFile! FIXME
// func (t *Tinzenite) sendFile(address, )

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
		log.Println("Merge: can not check if content is same!")
	} else {
		if stin.Content == msg.Object.Content {
			// log.Println("Core:", "Merge not required as updates are in sync!")
			// so all we need to do is apply the version update
			return t.model.ApplyModify(relPath, &msg.Object)
		}
	}
	// second: move to new name
	err = os.Rename(relPath.FullPath(), relPath.FullPath()+LOCAL)
	if err != nil {
		log.Println("Merge: original can not be found!")
		return err
	}
	// third: apply create of local version
	localVersionPath := relPath.RenameLastElement(relPath.LastElement() + LOCAL)
	err = t.model.ApplyCreate(localVersionPath, nil)
	if err != nil {
		log.Println("Merge: creating local merge file failed!")
		return err
	}
	// fourth: remove original
	err = t.model.ApplyRemove(relPath, nil)
	if err != nil {
		log.Println("Merge: removing original failed!")
		return err
	}
	// fifth: change path and apply remote as create
	msg.Operation = shared.OpCreate
	msg.Object.Path = relPath.SubPath() + REMOTE
	msg.Object.Name = relPath.LastElement() + REMOTE
	oldID := msg.Object.Identification
	msg.Object.Identification, err = shared.NewIdentifier()
	if err != nil {
		log.Println("Merge: failed to create new identifier!")
		return err
	}
	// new id --> rename temp file
	tempPath := t.Path + "/" + shared.TINZENITEDIR + "/" + shared.TEMPDIR
	err = os.Rename(tempPath+"/"+oldID, tempPath+"/"+msg.Object.Identification)
	if err != nil {
		log.Println("Merge: ipdating remote object file failed!")
		return err
	}
	// sixth: create remote file
	err = t.model.ApplyCreate(relPath.Apply(relPath.FullPath()+REMOTE), &msg.Object)
	if err != nil {
		log.Println("Merge: creating remote merge file failed!")
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
	defer func() { log.Println("Tinzenite: Background process stopped.") }()
	// timer for long transfer feedback
	transferTicker := time.Tick(5 * time.Second)
	// timer for peer management
	peerTicker := time.Tick(10 * time.Second)
	for {
		select {
		case <-t.stop:
			t.wg.Done()
			return
		case <-peerTicker:
			// update peers
			err := t.applyPeers()
			if err != nil {
				log.Println("Tin: error applying peers:", err)
			}
		case <-transferTicker:
			currentTransfers := t.channel.ActiveTransfers()
			// if currently none, done
			if len(currentTransfers) == 0 {
				continue
			}
			// find active transfer
			var currentProgress int
			for _, progress := range currentTransfers {
				if progress != 0 {
					currentProgress = progress
					break
				}
			}
			log.Printf("Tin: Pending %d transfers, current one at %d%%.\n", len(currentTransfers), currentProgress)
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
				name := msg.Object.Name
				// for better visibility add special mark to signify directory
				if msg.Object.Directory {
					name += "/++"
				}
				log.Printf("Tin: sending <%s> of <.../%s> to %s.\n", msg.Operation, name, address[:8])
				t.send(address, msg.JSON())
			}
		} // select
	} // for
}
