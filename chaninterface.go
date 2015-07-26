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
chaninterface implements the channel.Callbacks interface so that Tinzenite doesn't
export them unnecessarily.
*/
type chaninterface struct {
	// reference back to Tinzenite
	tin *Tinzenite
	// map of transfer objects, referenced by the object id
	transfers map[string]transfer
	// active stores all active file transfers so that we avoid getting multiple files from one peer at once
	active   map[string]bool
	recpath  string
	temppath string
	// stores address of peers we need to bootstrap
	bootstrap map[string]bool
}

func createChannelInterface(t *Tinzenite) *chaninterface {
	tempList := make(map[string]bool)
	path := t.Path + "/" + shared.TINZENITEDIR + "/" + shared.LOCALDIR + "/" + shared.BOOTJSON
	_, err := os.Lstat(path)
	if err == nil {
		data, err := ioutil.ReadFile(path)
		if err == nil {
			err := json.Unmarshal(data, &tempList)
			if err != nil {
				log.Println("Load:", err)
			}
		}
	}
	return &chaninterface{
		tin:       t,
		transfers: make(map[string]transfer),
		active:    make(map[string]bool),
		recpath:   t.Path + "/" + shared.TINZENITEDIR + "/" + shared.RECEIVINGDIR,
		temppath:  t.Path + "/" + shared.TINZENITEDIR + "/" + shared.TEMPDIR,
		bootstrap: tempList}
}

type transfer struct {
	// peers stores the addresses of all known peers that have the file update
	peers []string
	// the message to apply once the file has been received
	success shared.UpdateMessage
}

/*
Store saves the bootstrap list so that it remains active over disconnects.
*/
func (c *chaninterface) Store(root string) error {
	data, err := json.MarshalIndent(c.bootstrap, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(root+"/"+shared.TINZENITEDIR+"/"+shared.LOCALDIR+"/"+shared.BOOTJSON, data, shared.FILEPERMISSIONMODE)
}

/*
RequestFile is to be called by Tinzenite to authorize a file transfer
and to store what is to be done once it is successful. Handles multiplexing of
transfers as well. NOTE: Not a callback method.
*/
func (c *chaninterface) fetchAttachedFile(address string, um shared.UpdateMessage) {
	ident := um.Object.Identification
	if tran, exists := c.transfers[ident]; exists {
		// check if we need to update updatemessage
		oldVersion := tran.success.Object.Version
		newVersion := um.Object.Version
		if newVersion.Max() > oldVersion.Max() {
			/*TODO restart file transfer if applicable... oO*/
			/*TODO do I kick the old peer too?*/
			tran.success = um
			c.transfers[ident] = tran
			log.Println("Updated transfer!")
		}
		// add peer to available possibilities
		tran.peers = append(tran.peers, address)
		log.Println("Transfer already exists, added peer!")
		return
	}
	// create new one
	tran := transfer{peers: []string{address}, success: um}
	// add
	c.transfers[ident] = tran
	/*TODO send request to only one underutilized peer at once*/
	// FOR NOW: just get it from whomever send the update
	reqMsg := shared.CreateRequestMessage(shared.ReObject, ident)
	c.tin.channel.Send(address, reqMsg.String())
}

/*
Connect sends a connection request and prepares for bootstrapping. NOTE: Not a
callback method.
*/
func (c *chaninterface) Connect(address string) error {
	// send own peer
	msg, err := json.Marshal(c.tin.selfpeer)
	if err != nil {
		return err
	}
	// send request
	err = c.tin.channel.RequestConnection(address, string(msg))
	if err != nil {
		return err
	}
	// if request is sent successfully, remember for bootstrap
	// format to legal address
	address = strings.ToLower(address)[:64]
	c.bootstrap[address] = true
	return nil
}

// -------------------------CALLBACKS-------------------------------------------

/*
OnAllowFile is the callback that checks whether the transfer is to be accepted or
not. Checks the address and identification of the object against c.transfers.
*/
func (c *chaninterface) OnAllowFile(address, identification string) (bool, string) {
	tran, exists := c.transfers[identification]
	if !exists {
		log.Println("Transfer not authorized for", identification, "!")
		return false, ""
	}
	if !shared.Contains(tran.peers, address) {
		log.Println("Peer not authorized for transfer!")
		return false, ""
	}
	// here accept transfer
	// log.Printf("Allowing file <%s> from %s\n", identification, address)
	// add to active
	c.active[address] = true
	// name is address.identification to allow differentiating between same file from multiple peers
	filename := address + "." + identification
	return true, c.recpath + "/" + filename
}

/*
callbackFileReceived is for channel. It is called once the file has been successfully
received, thus initiates the actual local merging into the model.
*/
func (c *chaninterface) OnFileReceived(address, path, filename string) {
	// always free peer here
	delete(c.active, address)
	// split filename to get identification
	check := strings.Split(filename, ".")[0]
	identification := strings.Split(filename, ".")[1]
	if check != address {
		log.Println("Filename is mismatched!")
		return
	}
	if identification == MODEL {
		/*TODO if model --> call model.Syncmodel*/
		log.Println("TODO: CALL MODEL SYNC")
		/*TODO do we need to remove the transfer? Don't think so but check...*/
		return
	}
	/*TODO check request if file is delta / must be decrypted before applying to model*/
	tran, exists := c.transfers[identification]
	if !exists {
		log.Println("Transfer doesn't even exist anymore! Something bad went wrong...")
		// remove from transfers
		delete(c.transfers, identification)
		// remove any broken remaining temp files
		err := os.Remove(c.recpath + "/" + filename)
		if err != nil {
			log.Println("Failed to remove broken transfer file: " + err.Error())
		}
		return
	}
	// move from receiving to temp
	err := os.Rename(c.recpath+"/"+filename, c.temppath+"/"+identification)
	if err != nil {
		log.Println("Failed to move file to temp: " + err.Error())
		return
	}
	// apply
	err = c.applyUpdateWithMerge(tran.success)
	if err != nil {
		log.Println("File application error: " + err.Error())
	}
	// aaaand done!
}

/*
CallbackNewConnection is called when a new connection request comes in.
*/
func (c *chaninterface) OnNewConnection(address, message string) {
	if c.tin.peerValidation == nil {
		log.Println("PeerValidation() callback is unimplemented, can not connect!")
		return
	}
	// trusted peer flag
	var trusted bool
	// try to read peer from message
	peer := &shared.Peer{}
	err := json.Unmarshal([]byte(message), peer)
	if err != nil {
		// this may happen for debug purposes etc
		peer = nil
		trusted = false
		log.Println("Received non JSON message:", message)
	} else {
		trusted = true
	}
	// check if allowed
	/*TODO peer.trusted should be used to ensure that all is okay. For now all are trusted by default until encryption is implemented.*/
	if !c.tin.peerValidation(address, trusted) {
		log.Println("Refusing connection.")
		return
	}
	// if yes, add connection
	err = c.tin.channel.AcceptConnection(address)
	if err != nil {
		log.Println("Channel:", err)
		return
	}
	if peer == nil {
		log.Println("No legal peer information could be read! Peer will be considered passive.")
		return
	}
	// ensure that address is correct by overwritting sent address with real one
	peer.Address = address
	// add peer to local list
	c.tin.allPeers = append(c.tin.allPeers, peer)
	// try store new peer to disk
	_ = c.tin.Store()
}

/*
OnConnected is called when a peer comes online. We check whether it requires
bootstrapping, if not we do nothing.

TODO: this is not called on friend request! FIXME: Maybe by implementing a special
message?
*/
func (c *chaninterface) OnConnected(address string) {
	_, exists := c.bootstrap[address]
	if !exists {
		// nope, doesn't need bootstrap
		return
	}
	// initiate file transfer for peer obj
	rm := shared.CreateRequestMessage(shared.RePeer, "")
	c.tin.channel.Send(address, rm.String())
	// what happens: other peer sends update message as if new file, resulting in the peer being created
}

/*
CallbackMessage is called when a message is received.
*/
func (c *chaninterface) OnMessage(address, message string) {
	// find out type of message
	v := &shared.Message{}
	err := json.Unmarshal([]byte(message), v)
	if err == nil {
		switch msgType := v.Type; msgType {
		case shared.MsgUpdate:
			msg := &shared.UpdateMessage{}
			err := json.Unmarshal([]byte(message), msg)
			if err != nil {
				log.Println(err.Error())
				return
			}
			log.Println("Received update message!", msg.Operation)
			c.onUpdateMessage(address, *msg)
		case shared.MsgRequest:
			// read request message
			msg := &shared.RequestMessage{}
			err := json.Unmarshal([]byte(message), msg)
			if err != nil {
				log.Println(err.Error())
				return
			}
			if msg.Request == shared.ReModel {
				log.Println("Received model message!")
				c.onModelMessage(address, *msg)
			} else {
				log.Println("Received request message!")
				c.onRequestMessage(address, *msg)
			}
		default:
			log.Printf("Unknown object sent: %s!\n", msgType)
		}
		// If it was JSON, we're done if we can't do anything with it
		return
	}
	// if unmarshal didn't work check for plain commands:
	switch message {
	case "showrequest":
		obj, _ := c.tin.model.GetInfo(shared.CreatePath(c.tin.Path, "Damned Society - Sunny on Sunday.mp3"))
		rm := shared.CreateRequestMessage(shared.ReObject, obj.Identification)
		c.tin.send(address, rm.String())
	case "showmodelrequest":
		rm := shared.CreateRequestMessage(shared.ReModel, "model")
		c.tin.send(address, rm.String())
	case "showpeerupdate":
		/*NOTE: Will only work as long as that peer file exists. For bootstrap testing only!*/
		/*TODO: file name != ident... how do I fix this?*/
		fullPath := shared.CreatePath(c.tin.model.Root, "58f9432a9540f536.json")
		obj, err := c.tin.model.GetInfo(fullPath)
		if err != nil {
			log.Println("Model:", err)
			return
		}
		// update path to point correctly
		obj.Path = shared.TINZENITEDIR + "/" + shared.ORGDIR + "/" + shared.PEERSDIR + "/" + obj.Name
		// random identifier (would oc actually be real one, but for now...)
		obj.Identification, _ = shared.NewIdentifier()
		um := shared.CreateUpdateMessage(shared.OpCreate, *obj)
		c.tin.channel.Send(address, um.String())
	case "showremove":
		obj, _ := c.tin.model.GetInfo(shared.CreatePath(c.tin.Path, "remove.me"))
		um := shared.CreateUpdateMessage(shared.OpRemove, *obj)
		c.tin.channel.Send(address, um.String())
	default:
		log.Println("Received", message)
		c.tin.channel.Send(address, "ACK")
	}
}

func (c *chaninterface) onUpdateMessage(address string, msg shared.UpdateMessage) {
	if op := msg.Operation; op == shared.OpCreate || op == shared.OpModify {
		// create & modify must first fetch file
		c.fetchAttachedFile(address, msg)
	} else if op == shared.OpRemove {
		// remove is without file transfer, so directly apply
		c.applyUpdateWithMerge(msg)
	} else {
		log.Println("Unknown operation received, ignoring update message!")
	}
}

func (c *chaninterface) onRequestMessage(address string, msg shared.RequestMessage) {
	// this means we need to send our selfpeer
	if msg.Request == shared.RePeer {
		// so build a bogus update message and send that
		peerPath := shared.TINZENITEDIR + "/" + shared.ORGDIR + "/" + shared.PEERSDIR + "/" + c.tin.selfpeer.Identification + shared.ENDING
		fullPath := shared.CreatePath(c.tin.model.Root, peerPath)
		obj, err := c.tin.model.GetInfo(fullPath)
		if err != nil {
			log.Println("Model:", err)
			return
		}
		/*TODO maybe this should be modify?*/
		um := shared.CreateUpdateMessage(shared.OpCreate, *obj)
		c.tin.channel.Send(address, um.String())
		return
	}
	// get full path from model
	path, err := c.tin.model.FilePath(msg.Identification)
	if err != nil {
		log.Println("Model: ", err)
		return
	}
	err = c.tin.channel.SendFile(address, path, msg.Identification)
	if err != nil {
		log.Println(err)
	}
}

func (c *chaninterface) onModelMessage(address string, msg shared.RequestMessage) {
	// get model
	objModel, err := c.tin.model.Read()
	if err != nil {
		log.Println(err)
		return
	}
	// to JSON
	data, err := json.MarshalIndent(objModel, "", "  ")
	if err != nil {
		log.Println(err)
		return
	}
	filename := address + MODEL
	// write to file in temporary
	err = ioutil.WriteFile(c.tin.Path+"/"+shared.TINZENITEDIR+"/"+shared.TEMPDIR+"/"+filename, data, shared.FILEPERMISSIONMODE)
	if err != nil {
		log.Println(err)
		return
	}
	// send model as file
	err = c.tin.channel.SendFile(address, c.tin.Path+"/"+shared.TINZENITEDIR+"/"+shared.TEMPDIR+"/"+filename, filename)
	if err != nil {
		log.Println(err)
		return
	}
	/*TODO what to do when sent? need to remove temp file!*/
}

/*
applyUpdateWithMerge does exactly that. First it tries to apply the update. If it
fails with a merge a merge is done.
*/
func (c *chaninterface) applyUpdateWithMerge(msg shared.UpdateMessage) error {
	err := c.tin.model.ApplyUpdateMessage(&msg)
	if err == shared.ErrConflict {
		err := c.tin.merge(&msg)
		if err != nil {
			return err
		}
	}
	return nil
}
