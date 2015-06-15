package core

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
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
	Peers   []peer
}

/*
CreateContext creates a new Context attached to the given path.
*/
func CreateContext(name, dirpath string) (*Context, error) {
	// make sure it isn't already a Tinzenite directory
	if IsTinzenite(dirpath) {
		return nil, ErrIsTinzenite
	}
	var context Context
	context.Name = name
	context.Path = dirpath
	// init Tox
	options := &gotox.Options{
		true, true,
		gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0,
		3389,
		gotox.TOX_SAVEDATA_TYPE_NONE, nil}
	err := context.startTox(options)
	if err != nil {
		return nil, err
	}
	// set these otherwise Tox won't start
	context.tox.SelfSetName(name)
	context.tox.SelfSetStatusMessage("robot")
	// now that tox is running we can build the context struct
	address, err := context.tox.SelfGetAddress()
	if err != nil {
		return nil, err
	}
	context.Address = hex.EncodeToString(address)
	saveData, err := context.tox.GetSavedata()
	if err != nil {
		return nil, err
	}
	context.ToxData = saveData
	context.loadPeers()
	// and finally: store it (can we do this asynchroniously)
	err = context.Store()
	if err != nil {
		return nil, err
	}
	err = context.tox.Iterate()
	if err != nil {
		return nil, err
	}
	return &context, nil
}

/*
LoadContext loads a Context for the given path if it exists.
*/
func LoadContext(dirpath string) (*Context, error) {
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
	err = context.startTox(options)
	if err != nil {
		return nil, err
	}
	err = context.tox.Iterate()
	if err != nil {
		return nil, err
	}
	context.loadPeers()
	return context, nil
}

/*
Store all important informaton.
*/
func (context *Context) Store() error {
	var test uint32
	for {
		flag, _ := context.tox.FriendExists(test)
		if flag {
			// todo
			log.Printf("Friend %d exists!\n", test)
			test++
		} else {
			log.Println("Nope!")
			break
		}
	}
	// get the current save data to store (otherwise we may lose data)
	saveData, err := context.tox.GetSavedata()
	if err != nil {
		return err
	}
	context.ToxData = saveData
	// and finally: store it (can we do this asynchroniously)
	return saveContext(context)
}

func (context *Context) onFriendRequest(t *gotox.Tox, publicKey []byte, message string) {
	fmt.Printf("New friend request from %s\n", hex.EncodeToString(publicKey))
	fmt.Printf("With message: %v\n", message)
	context.addPeer("todo", publicKey)
}

func (context *Context) onFriendMessage(t *gotox.Tox, friendnumber uint32, messagetype gotox.ToxMessageType, message string) {
	if messagetype == gotox.TOX_MESSAGE_TYPE_NORMAL {
		fmt.Printf("New message from %d : %s\n", friendnumber, message)
		t.FriendSendMessage(friendnumber, messagetype, message)
	} else {
		fmt.Printf("New action from %d : %s\n", friendnumber, message)
	}
}

func (context *Context) onFriendStatus(tox *gotox.Tox, friendnumber uint32, userstatus gotox.ToxUserStatus) {
	fmt.Printf("Status of %d is %T\n", friendnumber, userstatus)
}

/*
Starts a Tox instance.
*/
func (context *Context) startTox(options *gotox.Options) error {
	tox, err := gotox.New(options)
	if err != nil {
		return err
	}
	err = tox.SelfSetStatus(gotox.TOX_USERSTATUS_NONE)
	// Register our callbacks
	tox.CallbackFriendRequest(context.onFriendRequest)
	tox.CallbackFriendMessage(context.onFriendMessage)
	tox.CallbackFriendStatusChanges(context.onFriendStatus)
	// Bootstrap
	toxNode, err := toxdynboot.FetchFirstAlive(100 * time.Millisecond)
	if err != nil {
		return err
	}
	err = tox.Bootstrap(toxNode.IPv4, toxNode.Port, toxNode.PublicKey)
	if err != nil {
		return err
	}
	context.tox = tox
	return nil
}

func (context *Context) addPeer(name string, address []byte) {
	log.Println("Added " + name + "!")
	_, err := context.tox.FriendAddNorequest(address)
	if err != nil {
		log.Println(err.Error())
		return
	}
	context.Peers = append(context.Peers, *CreatePeer(name, hex.EncodeToString(address)))
}

func (context *Context) loadPeers() {
	if context.tox == nil {
		log.Println("Tox is nil, called loadPeers too early!")
		return
	}
	for _, peer := range context.Peers {
		value, _ := hex.DecodeString(peer.Address)
		_, err := context.tox.FriendAddNorequest(value)
		if err != nil {
			log.Println(err.Error())
		}
	}
	log.Printf("Loaded %d peers!\n", len(context.Peers))
}
