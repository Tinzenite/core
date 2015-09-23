package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"

	"github.com/tinzenite/shared"
)

/*
onTrustedMessage is called for message from authenticated and trusted peers. Will
redestribute the message according to its type.
*/
func (c *chaninterface) onTrustedMessage(address string, msgType shared.MsgType, message string) {
	switch msgType {
	case shared.MsgUpdate:
		msg := &shared.UpdateMessage{}
		err := json.Unmarshal([]byte(message), msg)
		if err != nil {
			log.Println(err.Error())
			return
		}
		// handle the message and show log if error
		err = c.handleMessage(address, msg)
		if err != nil {
			c.log("handleMessage failed with:", err.Error())
		}
	case shared.MsgRequest:
		// read request message
		msg := &shared.RequestMessage{}
		err := json.Unmarshal([]byte(message), msg)
		if err != nil {
			log.Println(err.Error())
			return
		}
		if msg.ObjType == shared.OtModel {
			// c.log("Received model message!")
			c.onRequestModelMessage(address, *msg)
		} else {
			c.onRequestMessage(address, *msg)
		}
	case shared.MsgNotify:
		msg := &shared.NotifyMessage{}
		err := json.Unmarshal([]byte(message), msg)
		if err != nil {
			log.Println(err.Error())
			return
		}
		c.onNotifyMessage(address, *msg)
	default:
		c.warn("Unknown object received:", msgType.String())
	}
}

func (c *chaninterface) onRequestMessage(address string, msg shared.RequestMessage) {
	// this means we need to send our selfpeer (used for bootstrapping)
	if msg.ObjType == shared.OtPeer {
		// TODO check if this is really still in use?
		log.Println("DEBUG: YES, this is still in use. Why? Bootstrap should have fixed this...")
		// so build a bogus update message and send that
		peerPath := shared.TINZENITEDIR + "/" + shared.ORGDIR + "/" + shared.PEERSDIR + "/" + c.tin.selfpeer.Identification + shared.ENDING
		fullPath := shared.CreatePath(c.tin.model.RootPath, peerPath)
		obj, err := c.tin.model.GetInfo(fullPath)
		if err != nil {
			c.log("onRequestMessage:", err.Error())
			return
		}
		um := shared.CreateUpdateMessage(shared.OpCreate, *obj)
		c.tin.channel.Send(address, um.JSON())
		return
	}
	// get obj for path and directory
	obj, err := c.tin.model.GetInfoFrom(msg.Identification)
	if err != nil {
		c.log("Failed to locate object for", msg.Identification)
		return
	}
	// make sure we don't try to send a directory
	if obj.Directory {
		// theoretically shouldn't happen, but better safe than sorry
		c.warn("request is for directory, ignoring!")
		return
	}
	// so send file
	err = c.sendFile(address, c.tin.model.RootPath+"/"+obj.Path, msg.Identification, nil)
	if err != nil {
		c.log("failed to send file:", err.Error())
	}
}

func (c *chaninterface) onRequestModelMessage(address string, msg shared.RequestMessage) {
	// quietly update model
	c.tin.muteFlag = true
	defer func() { c.tin.muteFlag = false }()
	err := c.tin.model.Update()
	if err != nil {
		c.log("model update failed:", err.Error())
		return
	}
	// get model
	objModel, err := c.tin.model.Read()
	if err != nil {
		c.log("model.Read():", err.Error())
		return
	}
	// to JSON
	data, err := json.MarshalIndent(objModel, "", "  ")
	if err != nil {
		c.log("Json:", err.Error())
		return
	}
	filename := address + MODEL
	// write to file in temporary
	err = ioutil.WriteFile(c.tin.Path+"/"+shared.TINZENITEDIR+"/"+shared.TEMPDIR+"/"+filename, data, shared.FILEPERMISSIONMODE)
	if err != nil {
		c.log("WriteFile:", err.Error())
		return
	}
	// need to remove temp independent of whether success or not
	removeTemp := func(success bool) {
		err := os.Remove(c.tin.Path + "/" + shared.TINZENITEDIR + "/" + shared.TEMPDIR + "/" + filename)
		if err != nil {
			c.log("RemoveTemp:", err.Error())
		}
	}
	// send model as file. NOTE: name that is sent is not filename but IDMODEL
	err = c.sendFile(address, c.tin.Path+"/"+shared.TINZENITEDIR+"/"+shared.TEMPDIR+"/"+filename, shared.IDMODEL, removeTemp)
	if err != nil {
		c.log("SendFile:", err.Error())
		return
	}
}

/*
onNotifyMessage is called when a NotifyMessage is received.
*/
func (c *chaninterface) onNotifyMessage(address string, nm shared.NotifyMessage) {
	// for now we're only interested in remove notifications
	if nm.Notify != shared.NoRemoved {
		c.warn("Notify for non-Remove operations not yet supported, ignoring!")
		return
	}
	// check if removal even exists
	path := c.tin.model.RootPath + "/" + shared.TINZENITEDIR + "/" + shared.REMOVEDIR + "/" + nm.Identification
	if exists, _ := shared.DirectoryExists(path); !exists {
		c.warn("Notify received for non existant removal, ignoring!")
		return
	}
	// get peer id of sender
	var peerID string
	for _, peer := range c.tin.peers {
		if peer.Address == address {
			peerID = peer.Identification
			break
		}
	}
	// if still empty we couldn't find it
	if peerID == "" {
		c.warn("Notify could not be applied: peer not found!")
		return
	}
	// receiving this means that the other peer already has removed the REMOVEDIR, so add peer ourselves
	c.tin.model.UpdateRemovalDir(nm.Identification, peerID)
}

/*
remoteUpdate is a conveniance wrapper that fetches a file from the address for
the given update and then applies it.
*/
func (c *chaninterface) remoteUpdate(address string, msg shared.UpdateMessage) {
	// sanity check
	if msg.Operation == shared.OpRemove || msg.Object.Directory {
		c.warn("remoteUpdate called with remove or with directory, ignoring!")
		return
	}
	// create & modify must first fetch file
	rm := shared.CreateRequestMessage(shared.OtObject, msg.Object.Identification)
	// request file and apply update on success
	c.requestFile(address, rm, func(address, path string) {
		// rename to correct name for model
		err := os.Rename(path, c.temppath+"/"+rm.Identification)
		if err != nil {
			c.log("Failed to move file to temp: " + err.Error())
			return
		}
		// apply
		err = c.mergeUpdate(msg)
		if err != nil {
			c.log("File application error: " + err.Error())
		}
		// done
	})
}

/*
onModelFileReceived is called whenever a normal model sync is supposed to be
applied.
*/
func (c *chaninterface) onModelFileReceived(address, path string) {
	// read model file and remove it
	data, err := ioutil.ReadFile(path)
	if err != nil {
		c.log("ReModel failed to read model from disk:", err.Error())
		return
	}
	err = os.Remove(path)
	if err != nil {
		log.Println("ReModel could not remove model file:", err)
		// not strictly critical so no return here
	}
	// unmarshal
	foreignModel := &shared.ObjectInfo{}
	err = json.Unmarshal(data, foreignModel)
	if err != nil {
		log.Println("ReModel failed to parse JSON:", err)
		return
	}
	// get difference in updates
	updateLists, err := c.tin.model.Sync(foreignModel)
	if err != nil {
		log.Println("ReModel could not sync models:", err)
		return
	}
	// pretend that the updatemessage came from outside here
	for _, um := range updateLists {
		err := c.handleMessage(address, um)
		if err != nil {
			c.log("handleMessage failed with:", err.Error())
		}
	}
}