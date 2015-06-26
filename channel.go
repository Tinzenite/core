package core

import (
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/codedust/go-tox"
	"github.com/xamino/tox-dynboot"
)

/*
Channel is a wrapper of the gotox wrapper that creates and manages the underlying Tox
instance.
*/
type Channel struct {
	tox *gotox.Tox
	/*TODO all callbacks will block, need to avoid that especially when user interaction is required*/
	callbacks Callbacks
}

// Callbacks for external wrapped access.
type Callbacks interface {
	/*CallbackNewConnection is called on a Tox friend request.*/
	CallbackNewConnection(address, message string)
	/*CallbackMessage is called on an incomming message.*/
	CallbackMessage(address, message string)
}

var wg sync.WaitGroup
var stop chan bool

/*
CreateChannel creates and starts a new tox channel that continously runs in the background
until this object is destroyed.
*/
func CreateChannel(name string, toxdata []byte, callbacks Callbacks) (*Channel, error) {
	if name == "" {
		return nil, errors.New("CreateChannel called with no name!")
	}
	var channel = &Channel{}
	var options *gotox.Options
	var err error

	if toxdata == nil {
		options = &gotox.Options{
			true, true,
			gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0, 0,
			gotox.TOX_SAVEDATA_TYPE_NONE, nil}
	} else {
		options = &gotox.Options{
			true, true,
			gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0, 0,
			gotox.TOX_SAVEDATA_TYPE_TOX_SAVE, toxdata}
	}
	channel.tox, err = gotox.New(options)
	if err != nil {
		return nil, err
	}
	if toxdata == nil {
		channel.tox.SelfSetName(name)
		channel.tox.SelfSetStatusMessage("Tin Peer")
	}
	err = channel.tox.SelfSetStatus(gotox.TOX_USERSTATUS_NONE)
	// Register our callbacks
	channel.tox.CallbackFriendRequest(channel.onFriendRequest)
	channel.tox.CallbackFriendMessage(channel.onFriendMessage)
	// Bootstrap
	toxNode, err := toxdynboot.FetchFirstAlive(100 * time.Millisecond)
	if err != nil {
		return nil, err
	}
	log.Println("Bootstrapping to " + toxNode.IPv4)
	err = channel.tox.Bootstrap(toxNode.IPv4, toxNode.Port, toxNode.PublicKey)
	if err != nil {
		return nil, err
	}
	// register callbacks
	channel.callbacks = callbacks
	// now to run it:
	wg.Add(1)
	stop = make(chan bool, 1)
	go channel.run()
	return channel, nil
}

// --- public methods here ---

/*
Close shuts down the channel.
*/
func (channel *Channel) Close() {
	// send stop signal
	stop <- false
	// wait for it to close
	wg.Wait()
	// kill tox
	channel.tox.Kill()
}

/*
Address of the Tox instance.
*/
func (channel *Channel) Address() (string, error) {
	address, err := channel.tox.SelfGetAddress()
	if err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(address)), nil
}

/*
ToxData returns the underlying current representation of the tox data. Can be
used to store a Tox instance to disk.
*/
func (channel *Channel) ToxData() ([]byte, error) {
	return channel.tox.GetSavedata()
}

/*
Send a message to the given peer address.
*/
func (channel *Channel) Send(address, message string) error {
	key, err := hex.DecodeString(address)
	if err != nil {
		return err
	}
	id, err := channel.tox.FriendByPublicKey(key)
	if err != nil {
		return err
	}
	// returns message ID but we currently don't use it
	_, err = channel.tox.FriendSendMessage(id, gotox.TOX_MESSAGE_TYPE_NORMAL, message)
	return err
}

/*
AcceptConnection accepts the given address as a connection partner.
*/
func (channel *Channel) AcceptConnection(address string) error {
	publicKey, err := hex.DecodeString(address)
	if err != nil {
		return err
	}
	// ignore friendnumber
	_, err = channel.tox.FriendAddNorequest(publicKey)
	return err
}

/*
RequestConnection sends a friend request to the given address with the sending
peer information as the message for bootstrapping.
*/
func (channel *Channel) RequestConnection(address string, self *Peer) error {
	publicKey, err := hex.DecodeString(address)
	if err != nil {
		return err
	}
	msg, err := self.JSON()
	if err != nil {
		return err
	}
	/*TODO does this block!?*/
	_, err = channel.tox.FriendAdd(publicKey, string(msg))
	return err
}

// --- private methods here ---

/*
run is the background go routine method that keeps the Tox instance iterating
until Close() is called.
*/
func (channel *Channel) run() {
	for {
		temp, _ := channel.tox.IterationInterval()
		/*TODO maybe this needs to be smaller?*/
		intervall := time.Duration(temp) * time.Millisecond
		select {
		case <-stop:
			wg.Done()
			return
		case <-time.Tick(intervall):
			err := channel.tox.Iterate()
			if err != nil {
				// TODO what do we do here? Can we cleanly close the channel and
				// catch the error further up?
				log.Println(err.Error())
			}
		} // select
	} // for
}

/*
onFriendRequest calls the appropriate callback, wrapping it sanely for our purposes.
*/
func (channel *Channel) onFriendRequest(t *gotox.Tox, publicKey []byte, message string) {
	if channel.callbacks != nil {
		channel.callbacks.CallbackNewConnection(hex.EncodeToString(publicKey), message)
	} else {
		log.Println("Error: callbacks are nil!")
	}
}

/*
onFriendMessage calls the appropriate callback, wrapping it sanely for our purposes.
*/
func (channel *Channel) onFriendMessage(t *gotox.Tox, friendnumber uint32, messagetype gotox.ToxMessageType, message string) {
	/*TODO make sensible*/
	if messagetype == gotox.TOX_MESSAGE_TYPE_NORMAL {
		if channel.callbacks != nil {
			publicKey, err := channel.tox.FriendGetPublickey(friendnumber)
			if err != nil {
				channel.callbacks.CallbackMessage("ADDRESS_ERROR", message)
			} else {
				channel.callbacks.CallbackMessage(hex.EncodeToString(publicKey), message)
			}
		} else {
			log.Println("Error: callbacks are nil!")
		}
	}
}
