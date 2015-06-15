package core

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
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
}

var wg sync.WaitGroup
var running bool

/*
CreateChannel creates and starts a new tox channel that continously runs in the background
until this object is destroyed.
*/
func CreateChannel(context *Context, toxdata []byte) (*Channel, error) {
	if context == nil {
		return nil, errors.New("CreateChannel called with nil context!")
	}
	var channel = &Channel{nil}
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
		channel.tox.SelfSetName(context.Name)
		channel.tox.SelfSetStatusMessage("Robot")
	}
	err = channel.tox.SelfSetStatus(gotox.TOX_USERSTATUS_NONE)
	// Register our callbacks
	channel.tox.CallbackFriendRequest(channel.onFriendRequest)
	channel.tox.CallbackFriendMessage(channel.onFriendMessage)
	channel.tox.CallbackFriendStatusChanges(channel.onFriendStatus)
	// Bootstrap
	toxNode, err := toxdynboot.FetchFirstAlive(100 * time.Millisecond)
	if err != nil {
		return nil, err
	}
	err = channel.tox.Bootstrap(toxNode.IPv4, toxNode.Port, toxNode.PublicKey)
	if err != nil {
		return nil, err
	}
	log.Println("Created channel...")
	wg.Add(1)
	running = true
	go channel.run()
	return channel, nil
}

// --- public methods here ---

/*
Close shuts down the channel.
*/
func (channel *Channel) Close() {
	log.Println("Closing channel...")
	running = false
	wg.Wait()
	channel.tox.Kill()
	log.Println("Closed.")
}

// --- private methods here ---

func (channel *Channel) run() {
	for running {
		time.Sleep(1 * time.Second)
		log.Println("Tick")
	}
	wg.Done()
}

func (channel *Channel) onFriendRequest(t *gotox.Tox, publicKey []byte, message string) {
	fmt.Printf("New friend request from %s\n", hex.EncodeToString(publicKey))
	fmt.Printf("With message: %v\n", message)
	channel.tox.FriendAddNorequest(publicKey)
}

func (channel *Channel) onFriendMessage(t *gotox.Tox, friendnumber uint32, messagetype gotox.ToxMessageType, message string) {
	if messagetype == gotox.TOX_MESSAGE_TYPE_NORMAL {
		fmt.Printf("New message from %d : %s\n", friendnumber, message)
		t.FriendSendMessage(friendnumber, messagetype, message)
	} else {
		fmt.Printf("New action from %d : %s\n", friendnumber, message)
	}
}

func (channel *Channel) onFriendStatus(tox *gotox.Tox, friendnumber uint32, userstatus gotox.ToxUserStatus) {
	// fmt.Printf("Status of %d is %T\n", friendnumber, userstatus)
}
