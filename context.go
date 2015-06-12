package core

import (
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/codedust/go-tox"
	"github.com/xamino/tox-dynboot"
)

// Errors for Context
var (
	ErrIsTinzenite  = errors.New("Already a Tinzenite directory!")
	ErrNotTinzenite = errors.New("Not a Tinzenite directory!")
)

// Context is the peer context that Core will work with.
type Context struct {
	Name    string
	Path    string
	Address string
	ToxData []byte
	tox     *gotox.Tox
}

/*
Create a new Context attached to the given path.
*/
func Create(name, dirpath string) (*Context, error) {
	// make sure it isn't already a Tinzenite directory
	if IsTinzenite(dirpath) {
		return nil, ErrIsTinzenite
	}
	// init Tox
	options := &gotox.Options{
		true, true,
		gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0,
		3389,
		gotox.TOX_SAVEDATA_TYPE_NONE, nil}
	tox, err := startTox(options)
	if err != nil {
		return nil, err
	}
	// set these otherwise Tox won't start
	tox.SelfSetName(name)
	tox.SelfSetStatusMessage("robot")
	// now that tox is running we can build the context struct
	var context Context
	context.Name = name
	context.Path = dirpath
	address, err := tox.SelfGetAddress()
	if err != nil {
		return nil, err
	}
	context.Address = hex.EncodeToString(address)
	saveData, err := tox.GetSavedata()
	if err != nil {
		return nil, err
	}
	context.ToxData = saveData
	context.tox = tox
	// and finally: store it (can we do this asynchroniously)
	err = context.Store()
	if err != nil {
		return nil, err
	}
	return &context, nil
}

/*
Load a Context for the given path if it exists.
*/
func Load(dirpath string) (*Context, error) {
	if !IsTinzenite(dirpath) {
		return nil, ErrNotTinzenite
	}
	var context *Context
	context, err := loadContext(dirpath)
	if err != nil {
		return nil, err
	}
	// init Tox
	options := &gotox.Options{
		true, true,
		gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0,
		3389,
		gotox.TOX_SAVEDATA_TYPE_TOX_SAVE, context.ToxData}
	tox, err := startTox(options)
	if err != nil {
		return nil, err
	}
	context.tox = tox
	return context, nil
}

/*
Store all important informaton.
*/
func (context *Context) Store() error {
	saveData, err := context.tox.GetSavedata()
	if err != nil {
		return err
	}
	context.ToxData = saveData
	// and finally: store it (can we do this asynchroniously)
	return saveContext(context)
}

/*
Starts a Tox instance.
*/
func startTox(options *gotox.Options) (*gotox.Tox, error) {
	tox, err := gotox.New(options)
	if err != nil {
		return nil, err
	}
	err = tox.SelfSetStatus(gotox.TOX_USERSTATUS_NONE)
	// Register our callbacks
	tox.CallbackFriendRequest(onFriendRequest)
	tox.CallbackFriendMessage(onFriendMessage)
	// Bootstrap
	toxNode, err := toxdynboot.FetchFirstAlive(100 * time.Millisecond)
	if err != nil {
		return nil, err
	}
	err = tox.Bootstrap(toxNode.IPv4, toxNode.Port, toxNode.PublicKey)
	return tox, err
}

func onFriendRequest(t *gotox.Tox, publicKey []byte, message string) {
	fmt.Printf("New friend request from %s\n", hex.EncodeToString(publicKey))
	fmt.Printf("With message: %v\n", message)
	// Auto-accept friend request
	t.FriendAddNorequest(publicKey)
}

func onFriendMessage(t *gotox.Tox, friendnumber uint32, messagetype gotox.ToxMessageType, message string) {
	if messagetype == gotox.TOX_MESSAGE_TYPE_NORMAL {
		fmt.Printf("New message from %d : %s\n", friendnumber, message)
		t.FriendSendMessage(friendnumber, messagetype, message)
	} else {
		fmt.Printf("New action from %d : %s\n", friendnumber, message)
	}
}
