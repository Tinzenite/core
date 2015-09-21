package core

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/tinzenite/channel"
	"github.com/tinzenite/model"
	"github.com/tinzenite/shared"
)

/*
SyncModel fetches and synchronizes a remote model.
*/
func (c *chaninterface) SyncModel(address string) {
	// create & modify must first fetch file
	rm := shared.CreateRequestMessage(shared.OtModel, shared.IDMODEL)
	// request file and apply update on success
	c.requestFile(address, rm, c.onModelFileReceived)
}

/*
requestFile requests the given request from the address and executes the function
when the transfer was successful or not. NOTE: only f may be nil.
*/
func (c *chaninterface) requestFile(address string, rm shared.RequestMessage, f onDone) error {
	// build key
	key := c.buildKey(address, rm.Identification)
	if tran, exists := c.inTransfers[key]; exists {
		if time.Since(tran.updated) > transferTimeout {
			c.log("Retransmiting transfer due to timeout.")
			// update
			tran.updated = time.Now()
			c.inTransfers[key] = tran
			// retransmit
			return c.tin.send(address, rm.JSON())
		}
		c.log("Ignoring multiple request for", rm.Identification)
		return nil
	}
	// create new transfer
	tran := transfer{
		updated: time.Now(),
		peers:   []string{address},
		done:    f}
	c.inTransfers[key] = tran
	/*TODO send request to only one underutilized peer at once*/
	// FOR NOW: just get it from whomever send the update
	return c.tin.send(address, rm.JSON())
}

/*
onAuthenticationMessage handles the reception of an AuthenticationMessage.
NOTE: this should be the only method that is allowed to send messages to
UNAUTHENTICATED peers!
*/
func (c *chaninterface) onAuthenticationMessage(address string, msg shared.AuthenticationMessage) {
	// since we need this in either case, do it only once
	receivedNumber, err := c.tin.auth.ReadAuthentication(&msg)
	if err != nil {
		log.Println("Logic: failed to read authentication:", err)
		return
	}
	// check if reply to sent challenge
	if number, exists := c.challenges[address]; exists {
		log.Println("DEBUG: CHECKING REPLY")
		// whatever happens we remove the note that we've sent a challenge: if not valid we'll need to send a new one anyway
		delete(c.challenges, address)
		// response should be one higher than stored number
		expected := number + 1
		if receivedNumber != expected {
			log.Println("Logic: authentication failed for", address[:8], ": expected", expected, "got", receivedNumber, "!")
			return
		}
		// if valid, set peer to authenticated
		_, exists := c.tin.peers[address]
		if !exists {
			log.Println("Logic: peer lookup failed, doesn't exist!")
			return
		}
		// set value
		c.tin.peers[address].SetAuthenticated(true)
		// and done
		return
	}
	// if we didn't send a challenge, we just reply validly:
	log.Println("DEBUG: REPLYING")
	receivedNumber++
	// build reply
	reply, err := c.tin.auth.BuildAuthentication(receivedNumber)
	if err != nil {
		log.Println("Logic: failed to build response:", err)
		return
	}
	// send reply
	_ = c.tin.channel.Send(address, reply.JSON())
	// set the other peer to trusted (since they could send a valid challenge)
	_, exists := c.tin.peers[address]
	if !exists {
		log.Println("Logic: peer lookup failed, doesn't exist!")
		return
	}
	// set value
	c.tin.peers[address].SetAuthenticated(true)
	// and done!
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
		c.tin.send(address, um.JSON())
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
	if nm.Operation != shared.OpRemove {
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

/*
sendFile sends the given file to the address. Path is where the file lies,
identification is what it will be named in transfer, and the function will be
called once the send was successful.
*/
func (c *chaninterface) sendFile(address, path, identification string, f channel.OnDone) error {
	// key for keeping track of running transfers
	key := c.buildKey(address, identification)
	// we must wrap the function, even if none was given because we'll need to remove the outTransfers
	newFunction := func(success bool) {
		delete(c.outTransfers, key)
		// remember to call the callback
		if f != nil {
			f(success)
		} else if !success {
			// if no function was given still alert that send failed
			log.Println("Transfer was not successful!", path)
		}
	}
	// if it already exists, don't restart a new one!
	_, exists := c.outTransfers[key]
	if exists {
		// receiving side must restart if it so wants to, we'll just keep sending the original one
		return errors.New("out transfer already exists, will not resend")
	}
	// write that the transfer is happening
	c.outTransfers[key] = true
	// now call with overwritten function
	return c.tin.channel.SendFile(address, path, identification, newFunction)
}

/*
handleMessage looks at the message, fetches files if required, and correctly
applies it to the model.
*/
func (c *chaninterface) handleMessage(address string, msg *shared.UpdateMessage) error {
	// use check message to prepare message and check for special cases
	msg, err := c.tin.model.CheckMessage(msg)
	// if update known --> ignore it
	if err == model.ErrIgnoreUpdate {
		return nil
	}
	// if other side hasn't completed removal --> notify that we're done with it
	if err == model.ErrObjectRemovalDone {
		nm := shared.CreateNotifyMessage(shared.OpRemove, msg.Object.Name)
		c.tin.send(address, nm.JSON())
		// done
		return nil
	}
	// if still error, return it
	if err != nil {
		return err
	}
	// --> IF CheckMessage was ok, we can now handle applying the message
	// if a transfer was previously in progress, cancel it as we need the newer one
	key := c.buildKey(address, msg.Object.Identification)
	_, exists := c.inTransfers[key]
	if exists {
		path := c.recpath + "/" + address + "." + msg.Object.Identification
		err := c.tin.channel.CancelFileTransfer(path)
		// if canceling failed throw the error up
		if err != nil {
			return err
		}
		// remove transfer
		delete(c.inTransfers, key)
		// remove file if no error
		_ = os.Remove(path)
		// done with old one, so continue handling the new update
	}
	// apply directories directly
	if msg.Object.Directory {
		// no merge because it should never happen for directories
		return c.tin.model.ApplyUpdateMessage(msg)
	}
	op := msg.Operation
	// create and modify must first fetch the file
	if op == shared.OpCreate || op == shared.OpModify {
		c.remoteUpdate(address, *msg)
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
mergeUpdate does exactly that. First it tries to apply the update. If it fails
with a merge a merge is done.
*/
func (c *chaninterface) mergeUpdate(msg shared.UpdateMessage) error {
	// try to apply it straight
	err := c.tin.model.ApplyUpdateMessage(&msg)
	// if no error or not merge error, return err
	if err != shared.ErrConflict {
		return err
	}
	// if merge error --> merge
	return c.tin.merge(&msg)
}

/*
buildKey builds a unique string value for the given parameters.
*/
func (c *chaninterface) buildKey(address string, identification string) string {
	return address + ":" + identification
}

/*
Log function that respects the AllowLogging flag.
*/
func (c *chaninterface) log(msg ...string) {
	toPrint := []string{"ChanInterface:"}
	toPrint = append(toPrint, msg...)
	log.Println(strings.Join(toPrint, " "))
}

/*
Warn function.
*/
func (c *chaninterface) warn(msg ...string) {
	toPrint := []string{"ChanInterface:", "WARNING:"}
	toPrint = append(toPrint, msg...)
	log.Println(strings.Join(toPrint, " "))
}
