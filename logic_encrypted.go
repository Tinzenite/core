package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/tinzenite/model"
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
		c.requestFile(address, rm, c.encModelReceived)
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
		path = c.tin.Path + "/" + shared.TINZENITEDIR + "/" + shared.SENDINGDIR + "/" + shared.MODELJSON
		// get model info
		model, err := c.tin.model.Read()
		if err != nil {
			c.warn("Failed to read model:", err.Error())
			return
		}
		// convert to json
		data, err := json.Marshal(model)
		if err != nil {
			c.warn("Failed to marshal model to JSON:", err.Error())
			return
		}
		// write json to temp file
		err = ioutil.WriteFile(path, data, shared.FILEPERMISSIONMODE)
		if err != nil {
			c.warn("Failed to write model info to temp file:", err.Error())
			return
		}
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
	go c.encSendFile(address, msg.Identification, path, msg.ObjType)
	// TODO: shouldn't we reread the msg.ObjType from disk too?
}

/*
encSendFile handles uploading a file to the encrypted peer. This function is MADE
to run concurrently. Path is the path where the file CURRENTLY resides: the method
will copy all its data to SENDINGDIR, encrypt it there, and then send it.
*/
func (c *chaninterface) encSendFile(address, identification, path string, ot shared.ObjectType) {
	// read file data
	data, err := ioutil.ReadFile(path)
	if err != nil {
		c.warn("Failed to read data:", err.Error())
		return
	}
	// encrypt here as long as not auth AND not peer
	/*
		TODO enable encryption once everything works
		if ot != shared.OtAuth && ot != shared.OtPeer {
			data, err = c.tin.auth.Encrypt(data)
			if err != nil {
				c.warn("Failed to encrypt data!", err.Error())
				return
			}
		}
	*/
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
			c.log("encSendFile: Failed to upload file!", ot.String(), identification)
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
	// start by sending push for model
	pm = shared.CreatePushMessage(shared.IDMODEL, shared.OtModel)
	c.tin.channel.Send(address, pm.JSON())
	// then send a push for every file (not directories)
	for path, stin := range c.tin.model.StaticInfos {
		// if directory, skip
		if stin.Directory {
			continue
		}
		c.encSendPush(address, path, stin.Identification)
	}
	// and done
}

/*
encModelReceived is called when a model is received from an encrypted peer. It
triggers the complete sync with the encrypted state, concluding with updating
the encrypted peer to be up to date with this peer.
*/
func (c *chaninterface) encModelReceived(address, path string) {
	// no matter what: remove temp file
	defer func() {
		err := os.Remove(path)
		if err != nil {
			c.warn("encModelReceived: failed to remove received temp model file!", err.Error())
		}
	}()
	// read model
	data, err := ioutil.ReadFile(path)
	if err != nil {
		c.log("encModelReceived: failed to read received model file:", err.Error())
		return
	}
	// TODO decrypt file!
	// log.Println("DEBUG: TODO: decrypt model here!")
	// unmarshal
	foreignModel := &shared.ObjectInfo{}
	err = json.Unmarshal(data, foreignModel)
	if err != nil {
		c.log("encModelReceived: failed to parse JSON:", err.Error())
		return
	}
	// build path maps and associated object maps of foreign model
	foreignPaths := make(map[string]bool)
	foreignObjs := make(map[string]shared.ObjectInfo)
	foreignModel.ForEach(func(obj shared.ObjectInfo) {
		foreignPaths[obj.Path] = true
		// strip of children and write to objects
		obj.Objects = nil
		foreignObjs[obj.Path] = obj
	})
	// STEP ONE: get differences that THIS must get and apply from FOREIGN
	c.encApplyPeer(address, foreignPaths, foreignObjs)
	// STEP TWO: get difference that must be UPLOADED to foreign to make it equal to THIS
	c.encApplyLocal(address, foreignPaths, foreignObjs)
	// NOTE encrypted will be unlocked once all transfers are complete, see tinzenite.SyncEncrypted
	log.Println("DEBUG: done encrypted sync, awaiting transfer completion")
}

/*
handleEncryptedMessage looks at the message, fetches files if required, and correctly
applies it to the model. NOTE: blocks until file transfer has been applied or failed.
*/
func (c *chaninterface) handleEncryptedMessage(address string, msg *shared.UpdateMessage) error {
	// use check message to prepare message and check for special cases
	msg, err := c.tin.model.CheckMessage(msg)
	// if update known --> ignore it
	if err == model.ErrIgnoreUpdate {
		return nil
	}
	// if encrypted has a removal that we have registered as done, remove it
	if err == model.ErrObjectRemovalDone {
		nm := shared.CreateNotifyMessage(shared.NoRemoved, msg.Object.Identification)
		c.tin.channel.Send(address, nm.JSON())
		// done
		return nil
	}
	// if still error, return it
	if err != nil {
		return err
	}
	// --> IF CheckMessage was ok, we can now handle applying the message
	// apply directories directly
	if msg.Object.Directory {
		// no merge because it should never happen for directories
		return c.tin.model.ApplyUpdateMessage(msg)
	}
	op := msg.Operation
	// create and modify must first fetch the file
	if op == shared.OpCreate || op == shared.OpModify {
		rm := shared.CreateRequestMessage(shared.OtObject, msg.Object.Identification)
		var wg sync.WaitGroup
		wg.Add(1)
		c.requestFile(address, rm, func(address, path string) {
			// force calling function to wait until this has been handled
			defer func() { wg.Done() }()
			// rename to correct name for model
			err := os.Rename(path, c.temppath+"/"+rm.Identification)
			if err != nil {
				c.log("Failed to move file to temp: " + err.Error())
				return
			}
			// TODO decrypt file!
			// log.Println("DEBUG: TODO: decrypt file here!")
			// apply
			err = c.mergeUpdate(*msg)
			if err != nil {
				c.log("File application error: " + err.Error())
			}
			// done
		})
		// wait for file to be received before returning
		wg.Wait()
		// errors may turn up but only when the file has been received, so done here
		return nil
	} else if op == shared.OpRemove {
		// remove is without file transfer, so directly apply
		return c.mergeUpdate(*msg)
	}
	c.warn("Unknown operation received, ignoring update message!")
	return shared.ErrIllegalParameters
}

/*
encApplyPeer applies the foreign state of encrypted to the local model state by
fetching and applying everything.
*/
func (c *chaninterface) encApplyPeer(address string, foreignPaths map[string]bool, foreignObjs map[string]shared.ObjectInfo) {
	// get differences that foreignPaths may be ahead of
	created, remained, removed := shared.Difference(c.tin.model.TrackedPaths, foreignPaths)
	// we will wait until all updates have succesfully applied
	var wg sync.WaitGroup
	// all updates are applied with the same function, so reuse it
	apply := func(um shared.UpdateMessage) {
		defer func() { wg.Done() }() // no matter what unlock sync
		log.Println("DEBUG: doing", um.Operation, "for", um.Object.Path)
		err := c.handleEncryptedMessage(address, &um)
		if err != nil {
			c.log("encApplyPeer: handleEncryptedMessage: failed:", err.Error())
		}
	}
	for _, create := range created {
		// make sure not to try to create locally removed objects
		if c.tin.model.IsRemoved(foreignObjs[create].Identification) {
			continue
		}
		um := shared.CreateUpdateMessage(shared.OpCreate, foreignObjs[create])
		wg.Add(1)
		go apply(um)
	}
	for _, remains := range remained {
		// get local stin
		stin, exists := c.tin.model.StaticInfos[remains]
		if !exists {
			c.warn("encApplyPeer: object for modify check not found!")
			continue
		}
		// check if we must apply a modify
		if stin.Version.Includes(foreignObjs[remains].Version) {
			continue
		}
		um := shared.CreateUpdateMessage(shared.OpModify, foreignObjs[remains])
		wg.Add(1)
		go apply(um)
	}
	// wait for all created and modified to have been applied so that we can check if removals exist
	wg.Wait()
	for _, remove := range removed {
		// for remove we must use local object as no foreign one exists
		relPath := shared.CreatePath(c.tin.Path, remove)
		obj, err := c.tin.model.GetInfo(relPath)
		if err != nil {
			c.warn("encApplyPeer: object for removal not found:", err.Error())
			continue
		}
		// check if actually removed
		if !c.tin.model.IsRemoved(obj.Identification) {
			// if not this just means that enc doesn't know of a new object yet
			continue
		}
		// if not update as removal
		um := shared.CreateUpdateMessage(shared.OpRemove, *obj)
		wg.Add(1)
		go apply(um)
	}
	// wait until everything has been applied
	wg.Wait()
}

/*
encApplyLocal applies the local peer to the encrypted peer and sends the
required PushMessages.
*/
func (c *chaninterface) encApplyLocal(address string, foreignPaths map[string]bool, foreignObjs map[string]shared.ObjectInfo) {
	created, remained, removed := shared.Difference(foreignPaths, c.tin.model.TrackedPaths)
	// if no differences, we can immediately unlock and release the encryted peer
	if len(created) == 0 && len(remained) == 0 && len(removed) == 0 {
		log.Println("DEBUG: no changes to unlock, releasing immediately")
		_, exists := c.tin.peers[address]
		if !exists {
			c.warn("encApplyLocal: failed to preemptively release peer, doesn't exist!")
			// just return, may release later or timout
			return
		}
		ulm := shared.CreateLockMessage(shared.LoRelease)
		c.tin.channel.Send(address, ulm.JSON())
		c.tin.peers[address].SetLocked(false)
		// and done so return
		return
	}
	// for each path: check and create messages accordingly
	for _, create := range created {
		stin, exists := c.tin.model.StaticInfos[create]
		if !exists {
			// continue because this means it doesn't actually exist
			c.warn("encApplyLocal: missing stin for locally created object!")
			continue
		}
		if stin.Directory {
			continue
		}
		log.Println("Send push for created", create)
		c.encSendPush(address, create, stin.Identification)
	}
	for _, remains := range remained {
		stin, exists := c.tin.model.StaticInfos[remains]
		if !exists {
			// continue because this means it doesn't actually exist
			c.warn("encApplyLocal: missing stin for locally held object!")
			continue
		}
		// if dir ignore
		if stin.Directory {
			continue
		}
		// fetch foreign object
		fObj, exists := foreignObjs[remains]
		if !exists {
			c.warn("encApplyLocal: missing foreign object for locally held object!")
			continue
		}
		// if remote version already includes all known changes of local version, no need to send update, so continue
		if fObj.Version.Includes(stin.Version) {
			continue
		}
		// this means something has changed so reupload the object, overwritting the old version.
		log.Println("Send push for modified", remains)
		c.encSendPush(address, remains, stin.Identification)
	}
	// removed objects: use notify to have encrypted delete them
	for _, remove := range removed {
		stin, exists := foreignObjs[remove]
		if !exists {
			c.warn("encApplyLocal: missing stin from foreign objects!")
			continue
		}
		if stin.Directory {
			continue
		}
		log.Println("Send notify for removal of", remove)
		// TODO we may need more info than just the ID (peers?)
		nm := shared.CreateNotifyMessage(shared.NoRemoved, stin.Identification)
		c.tin.channel.Send(address, nm.JSON())
	}
	// and don't forget: update the model too!
	pm := shared.CreatePushMessage(shared.IDMODEL, shared.OtModel)
	c.tin.channel.Send(address, pm.JSON())
	// and done
}

/*
encSendPush sends a PushMessage for the given parameters, setting the ObjType
according to the path.
*/
func (c *chaninterface) encSendPush(address, path, identification string) {
	peerDir := shared.TINZENITEDIR + "/" + shared.ORGDIR + "/" + shared.PEERSDIR
	authPath := shared.TINZENITEDIR + "/" + shared.ORGDIR + "/" + shared.AUTHJSON
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
	pm := shared.CreatePushMessage(identification, objectType)
	c.tin.channel.Send(address, pm.JSON())
}
