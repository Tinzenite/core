package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/tinzenite/shared"
)

/*
onEncryptedMessage is called for messages from encrypted peers. Will redestribute
the message according to its type.

TODO describe order of operations (successful lock -> request model -> sync -> push / pull difference)
*/
func (c *chaninterface) onEncryptedMessage(address string, msgType shared.MsgType, message string) {
	// TODO switch and handle messages NOTE FIXME implement
	switch msgType {
	case shared.MsgLock:
		msg := &shared.LockMessage{}
		err := json.Unmarshal([]byte(message), msg)
		if err != nil {
			log.Println(err.Error())
			return
		}
		c.onEncLockMessage(address, *msg)
	case shared.MsgNotify:
		msg := &shared.NotifyMessage{}
		err := json.Unmarshal([]byte(message), msg)
		if err != nil {
			log.Println(err.Error())
			return
		}
		c.onEncNotifyMessage(address, *msg)
	default:
		c.warn("Unknown object received:", msgType.String())
	}
}

func (c *chaninterface) onEncLockMessage(address string, msg shared.LockMessage) {
	switch msg.Action {
	case shared.LoAccept:
		// remember that this peer is locked
		_, exists := c.tin.peers[address]
		if !exists {
			c.warn("Can not set peer to locked as peer doesn't exist!")
			return
		}
		c.tin.peers[address].SetLocked(true)
		// if LOCKED request model file to begin sync
		rm := shared.CreateRequestMessage(shared.OtModel, shared.IDMODEL)
		c.tin.channel.Send(address, rm.JSON())
	default:
		c.warn("Unknown lock action received:", msg.Action.String())
	}
}

/*
onEncNotifyMessage handles the reception of notification messages.
*/
func (c *chaninterface) onEncNotifyMessage(address string, msg shared.NotifyMessage) {
	switch msg.Notify {
	case shared.NoMissing:
		// remove transfer as no file will come
		delete(c.inTransfers, c.buildKey(address, msg.Identification))
		// if model --> create it
		if msg.Identification == shared.IDMODEL {
			// log that encrypted was empty and that we'll just upload our current state
			c.log("Encrypted is empty, nothing to merge, uploading directly.")
			c.doFullUpload(address)
			return
		}
		// if object --> error... maybe "reset" the encrypted peer?
		log.Println("DEBUG: something missing!", msg.Identification, msg.Notify)
	default:
		c.warn("Unknown notify type received:", msg.Notify.String())
	}
}

/*
doFullUpload uploads the current directory state to the encrypted peer. FIXME:
unlocks the encrypted peer once done.
*/
func (c *chaninterface) doFullUpload(address string) {
	// send model
	modelPath := c.tin.Path + "/" + shared.TINZENITEDIR + "/" + shared.LOCALDIR + "/" + shared.MODELJSON
	go c.encSend(address, shared.IDMODEL, modelPath, shared.OtModel)
	// for peers and auth file we require different objectTypes, so catch
	peerPath := shared.TINZENITEDIR + "/" + shared.ORGDIR + "/" + shared.PEERSDIR
	authPath := shared.TINZENITEDIR + "/" + shared.ORGDIR + "/" + shared.AUTHJSON
	// now, send every file based on the tracked objects in the model
	for path, stin := range c.tin.model.StaticInfos {
		// if directory, skip
		if stin.Directory {
			continue
		}
		// default object type is OtObject
		objectType := shared.OtObject
		// and set if path matches
		if strings.HasPrefix(path, peerPath) {
			objectType = shared.OtPeer
		}
		if strings.HasPrefix(path, authPath) {
			objectType = shared.OtAuth
		}
		// we do this concurrently because each call can take a while
		go c.encSend(address, stin.Identification, c.tin.Path+"/"+path, objectType)
	}
	// TODO when done with all upload, unlock peer! Can we unlock even though transfers are still running? WHERE do we unlock?
}

/*
encSend handles uploading a file to the encrypted peer. This function is MADE to
run concurrently. Path is the path where the file CURRENTLY resides: the method
will copy all its data to SENDINGDIR, encrypt it there, and then send it.
*/
func (c *chaninterface) encSend(address, identification, path string, ot shared.ObjectType) {
	// first send the push message so that it can be received while we work on preparing the file
	pm := shared.CreatePushMessage(identification, ot)
	// send push notify
	err := c.tin.channel.Send(address, pm.JSON())
	if err != nil {
		c.warn("Failed to send push message:", err.Error())
		return
	}
	// read file data
	data, err := ioutil.ReadFile(path)
	if err != nil {
		c.warn("Failed to read data:", err.Error())
		return
	}
	// TODO encrypt here? The time it takes serves as a time pause for allowing enc to handle the push message...
	// log.Println("DEBUG: encrypt here, and once done, send if time since timeout is larger!")
	// write to temp file
	sendPath := c.tin.Path + "/" + shared.TINZENITEDIR + "/" + shared.SENDINGDIR + "/" + identification
	err = ioutil.WriteFile(sendPath, data, shared.FILEPERMISSIONMODE)
	if err != nil {
		c.warn("Failed to write (encrypted) data to sending file:", err.Error())
		return
	}
	<-time.After(1 * time.Second)
	// send file
	err = c.tin.channel.SendFile(address, sendPath, identification, func(success bool) {
		if !success {
			c.log("Failed to upload file!", ot.String(), identification)
		}
		// remove sending temp file always
		err := os.Remove(sendPath)
		if err != nil {
			c.warn("Failed to remove sending file!", err.Error())
		}
	})
	if err != nil {
		c.warn("Failed to send file:", err.Error())
		return
	}
	// done
}
