package core

import (
	"encoding/hex"
	"encoding/json"
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

TODO all callbacks will block, need to avoid that especially when user interaction is required
*/
type Channel struct {
	tox       *gotox.Tox
	callbacks Callbacks
	wg        sync.WaitGroup
	stop      chan bool
}

// Callbacks for external wrapped access.
type Callbacks interface {
	/*CallbackNewConnection is called on a Tox friend request.*/
	callbackNewConnection(address, message string)
	/*CallbackMessage is called on an incomming message.*/
	callbackMessage(address, message string)
}

/*
CreateChannel creates and starts a new tox channel that continously runs in the background
until this object is destroyed.
*/
func CreateChannel(name string, toxdata []byte, callbacks Callbacks) (*Channel, error) {
	if name == "" {
		return nil, errors.New("CreateChannel called with no name!")
	}
	var init bool
	var channel = &Channel{}
	var options *gotox.Options
	var err error

	// this decides whether we are initiating a new connection or using an existing one
	if toxdata == nil {
		options = &gotox.Options{
			true, true,
			gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0, 0,
			gotox.TOX_SAVEDATA_TYPE_NONE, nil}
		init = true
	} else {
		options = &gotox.Options{
			true, true,
			gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0, 0,
			gotox.TOX_SAVEDATA_TYPE_TOX_SAVE, toxdata}
		init = false
	}
	channel.tox, err = gotox.New(options)
	if err != nil {
		return nil, err
	}
	if init {
		channel.tox.SelfSetName(name)
		channel.tox.SelfSetStatusMessage("Tin Peer")
	}
	err = channel.tox.SelfSetStatus(gotox.TOX_USERSTATUS_NONE)
	// Register our callbacks
	channel.tox.CallbackFriendRequest(channel.onFriendRequest)
	channel.tox.CallbackFriendMessage(channel.onFriendMessage)
	if init {
		// Bootstrap
		toxNode, err := toxdynboot.FetchFirstAlive(200 * time.Millisecond)
		if err != nil {
			return nil, err
		}
		err = channel.tox.Bootstrap(toxNode.IPv4, toxNode.Port, toxNode.PublicKey)
		if err != nil {
			return nil, err
		}
	}
	// register callbacks
	channel.callbacks = callbacks
	// now to run it:
	channel.wg.Add(1)
	channel.stop = make(chan bool, 1)
	go channel.run()
	return channel, nil
}

// --- public methods here ---

/*
Close shuts down the channel.
*/
func (channel *Channel) Close() {
	// send stop signal
	channel.stop <- false
	// wait for it to close
	channel.wg.Wait()
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
	msg, err := json.MarshalIndent(self, "", "  ")
	if err != nil {
		return err
	}
	// send non blocking friend request
	_, err = channel.tox.FriendAdd(publicKey, string(msg))
	return err
}

/*
IsOnline checks whether the given address is currently reachable.
*/
func (channel *Channel) IsOnline(address string) (bool, error) {
	publicKey, err := hex.DecodeString(address)
	if err != nil {
		return false, err
	}
	num, err := channel.tox.FriendByPublicKey(publicKey)
	if err != nil {
		return false, err
	}
	status, err := channel.tox.FriendGetConnectionStatus(num)
	if err != nil {
		return false, err
	}
	return status != gotox.TOX_CONNECTION_NONE, nil
}

/*
NameOf the key associated to the given address.
*/
func (channel *Channel) NameOf(address string) (string, error) {
	publicKey, err := hex.DecodeString(address)
	if err != nil {
		return "", err
	}
	num, err := channel.tox.FriendByPublicKey(publicKey)
	if err != nil {
		return "", err
	}
	name, err := channel.tox.FriendGetName(num)
	if err != nil {
		return "", err
	}
	return name, nil
}

// --- private methods here ---

/*
run is the background go routine method that keeps the Tox instance iterating
until Close() is called.
*/
func (channel *Channel) run() {
	for {
		temp, _ := channel.tox.IterationInterval()
		intervall := time.Duration(temp) * time.Millisecond
		select {
		case <-channel.stop:
			channel.wg.Done()
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
		channel.callbacks.callbackNewConnection(hex.EncodeToString(publicKey), message)
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
				channel.callbacks.callbackMessage("ADDRESS_ERROR", message)
			} else {
				channel.callbacks.callbackMessage(hex.EncodeToString(publicKey), message)
			}
		} else {
			log.Println("Error: callbacks are nil!")
		}
	}
}
