package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/tinzenite/channel"
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
		temppath:  t.Path + "/" + shared.TINZENITEDIR + "/" + shared.TEMPDIR}
}

type transfer struct {
	// peers stores the addresses of all known peers that have the file update
	peers []string
	// function to execute once the file has been received
	done onDone
}

type onDone func(address, path string)

/*
SyncModel fetches and synchronizes a remote model.
*/
func (c *chaninterface) SyncModel(address string) {
	// create & modify must first fetch file
	rm := shared.CreateRequestMessage(shared.ReModel, IDMODEL)
	// request file and apply update on success
	c.requestFile(address, rm, c.onModelFileReceived)
}

/*
requestFile requests the given request from the address and executes the function
when the transfer was successful or not. NOTE: only f may be nil.
*/
func (c *chaninterface) requestFile(address string, rm shared.RequestMessage, f onDone) error {
	// build key
	key := address + ":" + rm.Identification
	if _, exists := c.transfers[key]; exists {
		log.Println("TODO: IGNORING multiple request for", rm.Identification)
		/*TODO implement that if version higher cancel old and restart new, additional peers*/
		return shared.ErrUnsupported
	}
	// create new transfer
	tran := transfer{peers: []string{address}, done: f}
	c.transfers[key] = tran
	/*TODO send request to only one underutilized peer at once*/
	// FOR NOW: just get it from whomever send the update
	return c.tin.channel.Send(address, rm.String())
}

// -------------------------CALLBACKS-------------------------------------------

/*
OnAllowFile is the callback that checks whether the transfer is to be accepted or
not. Checks the address and identification of the object against c.transfers.
*/
func (c *chaninterface) OnAllowFile(address, identification string) (bool, string) {
	key := address + ":" + identification
	tran, exists := c.transfers[key]
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
	/*TODO check request if file is delta / must be decrypted before applying to model*/
	// get tran with key
	key := address + ":" + identification
	tran, exists := c.transfers[key]
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
	err := os.Rename(c.recpath+"/"+filename, c.temppath+"/"+filename)
	if err != nil {
		log.Println("Failed to move file to temp: " + err.Error())
		return
	}
	// execute done function if it exists
	if tran.done != nil {
		tran.done(address, c.temppath+"/"+filename)
	}
}

/*
CallbackNewConnection is called when a bootstrap request comes in. This means
that the OTHER peer is bootstrapping: all we need to do here is save the other's
peer information and include it in the network if allowed.
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
OnConnected is called whenever a peer comes online. Starts authentication process.
*/
func (c *chaninterface) OnConnected(address string) {
	log.Println(address, "came online!")
	/*TODO implement authentication!*/
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
				c.onRequestModelMessage(address, *msg)
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
		// fetch and apply file
		c.remoteUpdate(address, msg)
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
	err = c.sendFile(address, path, msg.Identification, nil)
	if err != nil {
		log.Println(err)
	}
}

func (c *chaninterface) onRequestModelMessage(address string, msg shared.RequestMessage) {
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
	// need to remove temp independent of whether success or not
	removeTemp := func(success bool) {
		err := os.Remove(c.tin.Path + "/" + shared.TINZENITEDIR + "/" + shared.TEMPDIR + "/" + filename)
		if err != nil {
			log.Println("RemoveTemp:", err)
		}
	}
	// send model as file. NOTE: name that is sent is not filename but IDMODEL
	err = c.sendFile(address, c.tin.Path+"/"+shared.TINZENITEDIR+"/"+shared.TEMPDIR+"/"+filename, IDMODEL, removeTemp)
	if err != nil {
		log.Println(err)
		return
	}
}

/*
remoteUpdate is a conveniance wrapper that fetches a file from the address for
the given update and then applies it.
*/
func (c *chaninterface) remoteUpdate(address string, msg shared.UpdateMessage) {
	// create & modify must first fetch file
	rm := shared.CreateRequestMessage(shared.ReObject, msg.Object.Identification)
	// request file and apply update on success
	c.requestFile(address, rm, func(address, path string) {
		// rename to correct name for model
		err := os.Rename(path, c.temppath+"/"+rm.Identification)
		if err != nil {
			log.Println("Failed to move file to temp: " + err.Error())
			return
		}
		// apply
		err = c.applyUpdateWithMerge(msg)
		if err != nil {
			log.Println("File application error: " + err.Error())
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
		log.Println("ReModel:", err)
		return
	}
	err = os.Remove(path)
	if err != nil {
		log.Println("ReModel:", err)
		// not strictly critical so no return here
	}
	// unmarshal
	foreignModel := &shared.ObjectInfo{}
	err = json.Unmarshal(data, foreignModel)
	if err != nil {
		log.Println("ReModel:", err)
		return
	}
	// get difference in updates
	updateLists, err := c.tin.model.SyncModel(foreignModel)
	if err != nil {
		log.Println("ReModel:", err)
		return
	}
	// pretend that the updatemessage came from outside here
	for _, um := range updateLists {
		c.remoteUpdate(address, *um)
	}
}

/*
sendFile ensures that only a limited number of transfers are running at the same
time.

TODO implement. Write to buffered channel? Channel reads from it? Or via queue?
Actually it seems like Tox does that by itself: https://github.com/irungentoo/toxcore/issues/1382
*/
func (c *chaninterface) sendFile(address, path, identification string, f channel.OnDone) error {
	// if no function is given we want to at least be notified if something went wrong, right?
	if f == nil {
		f = func(success bool) {
			if !success {
				log.Println("Send failed!")
			}
		}
	}
	return c.tin.channel.SendFile(address, path, identification, f)
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
