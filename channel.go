package core

import (
	"encoding/hex"
	"errors"
	"fmt"
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
type channel struct {
	tox *gotox.Tox
	/*TODO all callbacks will block, need to avoid that especially when user interaction is required*/
	callbacks callbacks
}

type callbacks interface {
	/*CallbackNewConnection is called on a Tox friend request.	 */
	callbackNewConnection(address, message string) bool
	/*CallbackMessage is called on an incomming message.*/
	callbackMessage(address, message string)
}

var wg sync.WaitGroup
var stop chan bool

/*
CreateChannel creates and starts a new tox channel that continously runs in the background
until this object is destroyed.
*/
func createChannel(name string, toxdata []byte, callbacks callbacks) (*channel, error) {
	if name == "" {
		return nil, errors.New("CreateChannel called with no name!")
	}
	var channel = &channel{}
	var options *gotox.Options
	var err error

	if toxdata == nil {
		options = &gotox.Options{
			true, true,
			gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0,
			3389,
			gotox.TOX_SAVEDATA_TYPE_NONE, nil}
	} else {
		options = &gotox.Options{
			true, true,
			gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0,
			3389,
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
	err = channel.tox.Bootstrap(toxNode.IPv4, toxNode.Port, toxNode.PublicKey)
	if err != nil {
		return nil, err
	}
	// register callbacks
	// channel.callback = callback
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
func (channel *channel) Close() {
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
func (channel *channel) Address() (string, error) {
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
func (channel *channel) ToxData() ([]byte, error) {
	return channel.tox.GetSavedata()
}

/*
Send a message to the given peer address.
*/
func (channel *channel) Send(address, message string) error {
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

// --- private methods here ---

/*
run is the background go routine method that keeps the Tox instance iterating
until Close() is called.
*/
func (channel *channel) run() {
	for {
		temp, _ := channel.tox.IterationInterval()
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
				panic(err)
			}
		} // select
	} // for
}

func (channel *channel) onFriendRequest(t *gotox.Tox, publicKey []byte, message string) {
	// if the callback returns true we are supposed to accept the connection
	/*TODO this blocks, how do I avoid that?*/
	if channel.callbacks.callbackNewConnection(hex.EncodeToString(publicKey), message) {
		channel.tox.FriendAddNorequest(publicKey)
	}
}

func (channel *channel) onFriendMessage(t *gotox.Tox, friendnumber uint32, messagetype gotox.ToxMessageType, message string) {
	if messagetype == gotox.TOX_MESSAGE_TYPE_NORMAL {
		fmt.Printf("New message from %d : %s\n", friendnumber, message)
		t.FriendSendMessage(friendnumber, messagetype, message)
		// get friend address
		publicKey, err := t.FriendGetPublickey(friendnumber)
		if err != nil {
			log.Println("Failed to find address! Not calling callback!")
			return
		}
		channel.callbacks.callbackMessage(hex.EncodeToString(publicKey), message)
	} else {
		fmt.Printf("New action from %d : %s\n", friendnumber, message)
	}
}