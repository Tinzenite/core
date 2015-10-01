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
	case shared.MsgRequest:
		msg := &shared.RequestMessage{}
		err := json.Unmarshal([]byte(message), msg)
		if err != nil {
			log.Println(err.Error())
			return
		}
		c.onEncRequestMessage(address, *msg)
	default:
		c.warn("Unknown object received:", msgType.String())
	}
}

/*
onEncLockMessage handles lock messages. Notably it requests a model on a
successful lock, starting a synchronization.
*/
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
	case shared.LoRelease:
		// unset lock of this peer
		_, exists := c.tin.peers[address]
		if !exists {
			c.warn("Can not release peer as peer doesn't exist!")
			return
		}
		c.tin.peers[address].SetLocked(false)
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
			// so send push for model and all objects
			c.sendCompletePushes(address)
			return
		}
		// TODO: if object --> error... maybe "reset" the encrypted peer?
		log.Println("DEBUG: object missing!", msg.Identification, msg.Notify)
		log.Println("DEBUG: TODO: reset encrypted peer?")
	default:
		c.warn("Unknown notify type received:", msg.Notify.String())
	}
}

/*
onEncRequestMessage handles the reception of a request message following a push
message. Triggers the sending of the requested file.
*/
func (c *chaninterface) onEncRequestMessage(address string, msg shared.RequestMessage) {
	var path string
	// if model has been requested --> path is different as not tracked itself
	if msg.Identification == shared.IDMODEL {
		path = c.tin.Path + "/" + shared.TINZENITEDIR + "/" + shared.LOCALDIR + "/" + shared.MODELJSON
	} else {
		// get subPath for file
		subPath, err := c.tin.model.GetSubPath(msg.Identification)
		if err != nil {
			c.warn("Failed to locate subpath for request message!", msg.Identification)
			return
		}
		path = c.tin.Path + "/" + subPath
	}
	// and send file (concurrent because of encryption)
	go c.encSend(address, msg.Identification, path, msg.ObjType)
	// TODO: shouldn't we reread the msg.ObjType from disk too?
}

/*
encSend handles uploading a file to the encrypted peer. This function is MADE to
run concurrently. Path is the path where the file CURRENTLY resides: the method
will copy all its data to SENDINGDIR, encrypt it there, and then send it.
*/
func (c *chaninterface) encSend(address, identification, path string, ot shared.ObjectType) {
	// read file data
	data, err := ioutil.ReadFile(path)
	if err != nil {
		c.warn("Failed to read data:", err.Error())
		return
	}
	// TODO encrypt here? FIXME
	// log.Println("DEBUG: encrypt here, and once done, send if time since timeout is larger!")
	// write to temp file
	sendPath := c.tin.Path + "/" + shared.TINZENITEDIR + "/" + shared.SENDINGDIR + "/" + identification
	err = ioutil.WriteFile(sendPath, data, shared.FILEPERMISSIONMODE)
	if err != nil {
		c.warn("Failed to write (encrypted) data to sending file:", err.Error())
		return
	}
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

/*
sendCompletePushes sends push models for everything, starting with the model.
This will result in the encrypted peer requesting all objects.
*/
func (c *chaninterface) sendCompletePushes(address string) {
	// vars we'll use
	var pm shared.PushMessage
	peerDir := shared.TINZENITEDIR + "/" + shared.ORGDIR + "/" + shared.PEERSDIR
	authPath := shared.TINZENITEDIR + "/" + shared.ORGDIR + "/" + shared.AUTHJSON
	// start by sending push for model
	pm = shared.CreatePushMessage(shared.IDMODEL, shared.OtModel)
	c.tin.channel.Send(address, pm.JSON())
	for path, stin := range c.tin.model.StaticInfos {
		// if directory, skip
		if stin.Directory {
			continue
		}
		// default object type is OtObject
		objectType := shared.OtObject
		// if peer --> update objectType
		if strings.HasPrefix(path, peerDir) {
			objectType = shared.OtPeer
		}
		// if auth file --> update objectType
		if path == authPath {
			objectType = shared.OtAuth
		}
		pm = shared.CreatePushMessage(stin.Identification, objectType)
		c.tin.channel.Send(address, pm.JSON())
	}
	// and done
}
