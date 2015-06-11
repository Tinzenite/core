package core

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/codedust/go-tox"
	"github.com/xamino/tox-dynboot"
)

/*
NewContext creates a new Context with the given name. It will generate the underlying
required ToxData and fill the information.
*/
func NewContext(name string) (*Context, error) {
	var context Context
	context.Name = name
	// all ok, build and kill tox instance so that keys are generated
	options := &gotox.Options{
		true, true,
		gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0,
		3389,
		gotox.TOX_SAVEDATA_TYPE_NONE, nil}
	tox, err := gotox.New(options)
	if err != nil {
		return nil, err
	}
	defer tox.Kill()
	// print address
	address, err := tox.SelfGetAddress()
	if err != nil {
		return nil, err
	}
	context.Address = hex.EncodeToString(address)
	// store save data
	savedata, err := tox.GetSavedata()
	if err != nil {
		return nil, err
	}
	context.ToxData = savedata
	return &context, nil
}

// Run starts an echoing Tox client for now.
func Run(context Context) {
	fmt.Println("Starting now...")
	// set options
	options := &gotox.Options{
		true, true,
		gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0,
		3389,
		gotox.TOX_SAVEDATA_TYPE_TOX_SAVE, context.ToxData}
	// create new
	tox, err := gotox.New(options)
	if err != nil {
		panic(err)
	}
	// defer the closing of tox
	defer tox.Kill()
	// set name
	tox.SelfSetName(context.Name)
	// print address
	fmt.Printf("Name: %s\nID: %s\n", context.Name, context.Address)

	err = tox.SelfSetStatus(gotox.TOX_USERSTATUS_NONE)

	// Register our callbacks
	tox.CallbackFriendRequest(onFriendRequest)
	tox.CallbackFriendMessage(onFriendMessage)

	// Bootstrap
	toxNode, err := toxdynboot.FetchFirstAlive(100 * time.Millisecond)
	if err != nil {
		log.Fatal(err.Error())
		return
	}
	err = tox.Bootstrap(toxNode.IPv4, toxNode.Port, toxNode.PublicKey)
	if err != nil {
		panic(err)
	}

	isRunning := true

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	// ticker := time.NewTicker(25 * time.Millisecond)

	for isRunning {
		temp, _ := tox.IterationInterval()
		intervall := time.Duration(temp) * time.Millisecond
		select {
		case <-c:
			fmt.Println("Killing")
			isRunning = false
		case <-time.Tick(intervall):
			tox.Iterate()
		}
	}
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
	} else {
		fmt.Printf("New action from %d : %s\n", friendnumber, message)
	}

	// Echo back
	t.FriendSendMessage(friendnumber, messagetype, message)
}
