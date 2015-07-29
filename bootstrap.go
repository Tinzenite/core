package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/tinzenite/shared"
)

/*
Connect sends a connection request and prepares for bootstrapping. NOTE: Not a
callback method.

TODO: add callback functionality here when bootstrap works
*/
func (c *chaninterface) StartBootstrap(address string) error {
	// send own peer
	msg, err := json.Marshal(c.tin.selfpeer)
	if err != nil {
		return err
	}
	// send request
	err = c.tin.channel.RequestConnection(address, string(msg))
	if err != nil {
		return err
	}
	// if request is sent successfully, remember for bootstrap
	// format to legal address
	address = strings.ToLower(address)[:64]
	c.bootstrap[address] = true
	return nil
}

/*
OnConnected is called when a peer comes online. We check whether it requires
bootstrapping, if not we do nothing.

TODO: this is not called on friend request! FIXME: Maybe by implementing a special
message?
*/
func (c *chaninterface) OnConnected(address string) {
	_, exists := c.bootstrap[address]
	if !exists {
		log.Println("Missing", address)
		// nope, doesn't need bootstrap
		return
	}
	// bootstrap
	rm := shared.CreateRequestMessage(shared.ReModel, IDMODEL)
	c.requestFile(address, rm, func(address, path string) {
		// read model file and remove it
		data, err := ioutil.ReadFile(path)
		if err != nil {
			log.Println("ReModel:", err)
			return
		}
		err = os.Remove(path)
		if err != nil {
			log.Println("ReModel:", err)
			// not strictly critical so no return here
		}
		// unmarshal
		foreignModel := &shared.ObjectInfo{}
		err = json.Unmarshal(data, foreignModel)
		if err != nil {
			log.Println("ReModel:", err)
			return
		}
		// get difference in updates
		var updateLists []*shared.UpdateMessage
		updateLists, err = c.tin.model.BootstrapModel(foreignModel)
		if err != nil {
			log.Println("ReModel:", err)
			return
		}
		// pretend that the updatemessage came from outside here
		for _, um := range updateLists {
			c.remoteUpdate(address, *um)
		}
		// bootstrap --> special behaviour, so call the finish method
		log.Println("Finish bootstrap here!")
	})
}

/*
CallbackNewConnection is called when a bootstrap request comes in. This means
that the OTHER peer is bootstrapping: all we need to do here is save the other's
peer information and include it in the network if allowed.
*/
func (c *chaninterface) OnNewConnection(address, message string) {
	if c.tin.peerValidation == nil {
		log.Println("PeerValidation() callback is unimplemented, can not connect!")
		return
	}
	// trusted peer flag
	var trusted bool
	// try to read peer from message
	peer := &shared.Peer{}
	err := json.Unmarshal([]byte(message), peer)
	if err != nil {
		// this may happen for debug purposes etc
		peer = nil
		trusted = false
		log.Println("Received non JSON message:", message)
	} else {
		trusted = true
	}
	// check if allowed
	/*TODO peer.trusted should be used to ensure that all is okay. For now all are trusted by default until encryption is implemented.*/
	if !c.tin.peerValidation(address, trusted) {
		log.Println("Refusing connection.")
		return
	}
	// if yes, add connection
	err = c.tin.channel.AcceptConnection(address)
	if err != nil {
		log.Println("Channel:", err)
		return
	}
	if peer == nil {
		log.Println("No legal peer information could be read! Peer will be considered passive.")
		return
	}
	// ensure that address is correct by overwritting sent address with real one
	peer.Address = address
	// add peer to local list
	c.tin.allPeers = append(c.tin.allPeers, peer)
	// try store new peer to disk
	_ = c.tin.Store()
}
