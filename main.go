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

// Run starts an echoing Tox client for now.
func Run() {
	fmt.Println("Starting now...")
	// set options
	options := &gotox.Options{
		true, true,
		gotox.TOX_PROXY_TYPE_NONE, "127.0.0.1", 5555, 0, 0,
		3389,
		gotox.TOX_SAVEDATA_TYPE_NONE, nil}
	// create new
	tox, err := gotox.New(options)
	if err != nil {
		panic(err)
	}
	// defer the closing of tox
	defer tox.Kill()
	// set name
	tox.SelfSetName("TestTox")
	// print address
	address, _ := tox.SelfGetAddress()
	fmt.Printf("%s\n", hex.EncodeToString(address))

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
	ticker := time.NewTicker(25 * time.Millisecond)

	for isRunning {
		select {
		case <-c:
			fmt.Println("Killing")
			isRunning = false
		case <-ticker.C:
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
