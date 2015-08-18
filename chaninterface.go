package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

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
	// map of transfer objects, referenced by the object id. Both for in and out.
	inTransfers  map[string]transfer
	outTransfers map[string]transfer
	// active stores all active file transfers so that we avoid getting multiple files from one peer at once
	active       map[string]bool
	recpath      string
	temppath     string
	AllowLogging bool
}

func createChannelInterface(t *Tinzenite) *chaninterface {
	return &chaninterface{
		tin:          t,
		inTransfers:  make(map[string]transfer),
		outTransfers: make(map[string]transfer),
		active:       make(map[string]bool),
		recpath:      t.Path + "/" + shared.TINZENITEDIR + "/" + shared.RECEIVINGDIR,
		temppath:     t.Path + "/" + shared.TINZENITEDIR + "/" + shared.TEMPDIR,
		AllowLogging: true}
}

type transfer struct {
	// start time of transfer for timeout reasons
	start time.Time
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
	rm := shared.CreateRequestMessage(shared.ReModel, shared.IDMODEL)
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
	if tran, exists := c.inTransfers[key]; exists {
		if time.Since(tran.start) > transferTimeout {
			// c.log("Retransmiting transfer.")
			// update
			tran.start = time.Now()
			c.inTransfers[key] = tran
			// retransmit
			return c.tin.channel.Send(address, rm.String())
		}
		// log.Println("TODO: IGNORING multiple request for", rm.Identification)
		/*TODO implement that if version higher cancel old and restart new, additional peers*/
		return shared.ErrUnsupported
	}
	// create new transfer
	tran := transfer{
		start: time.Now(),
		peers: []string{address},
		done:  f}
	c.inTransfers[key] = tran
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
	tran, exists := c.inTransfers[key]
	if !exists {
		c.log("Transfer not authorized for", identification, "!")
		return false, ""
	}
	if !shared.Contains(tran.peers, address) {
		c.log("Peer not authorized for transfer!")
		return false, ""
	}
	// check timeout
	if time.Since(tran.start) > transferTimeout {
		// c.log("Transfer timed out!")
		delete(c.inTransfers, key)
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
		c.log("Filename is mismatched!")
		return
	}
	/*TODO check request if file is delta / must be decrypted before applying to model*/
	// get tran with key
	key := address + ":" + identification
	tran, exists := c.inTransfers[key]
	if !exists {
		c.log("Transfer doesn't even exist anymore! Something bad went wrong...")
		// remove from transfers
		delete(c.inTransfers, identification)
		// remove any broken remaining temp files
		err := os.Remove(c.recpath + "/" + filename)
		if err != nil {
			c.log("Failed to remove broken transfer file: " + err.Error())
		}
		return
	}
	// remove transfer
	delete(c.inTransfers, key)
	// move from receiving to temp
	err := os.Rename(c.recpath+"/"+filename, c.temppath+"/"+filename)
	if err != nil {
		c.log("Failed to move file to temp: " + err.Error())
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
		c.warn("PeerValidation() callback is unimplemented, can not connect!")
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
		c.log("Received non JSON message:", message)
	} else {
		trusted = true
	}
	// check if allowed
	/*TODO peer.trusted should be used to ensure that all is okay. For now all are trusted by default until encryption is implemented.*/
	if !c.tin.peerValidation(address, trusted) {
		c.log("Refusing connection.")
		return
	}
	// if yes, add connection
	err = c.tin.channel.AcceptConnection(address)
	if err != nil {
		c.log("Channel:", err.Error())
		return
	}
	if peer == nil {
		c.warn("No legal peer information could be read! Peer will be considered passive.")
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
	c.log(address, "came online!")
	/*TODO implement authentication! Also in Bootstrap...*/
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
				// c.log("Received model message!")
				c.onRequestModelMessage(address, *msg)
			} else {
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
	default:
		c.log("Received", message)
		c.tin.channel.Send(address, "ACK")
	}
}

func (c *chaninterface) onUpdateMessage(address string, msg shared.UpdateMessage) {
	// check if we even have to apply it to avoid recursive requesting!
	if c.tin.model.HasUpdate(&msg) {
		// c.log("Update is known, ignoring!")
		return
	}
	c.log("Received update message!", msg.Operation.String(), msg.Object.Path)
	// if directory we don't want to request anything since we can directly apply it
	if msg.Object.Directory {
		c.applyUpdateWithMerge(msg)
		return
	}
	// HERE only files
	// apply / fetch and apply
	if op := msg.Operation; op == shared.OpCreate || op == shared.OpModify {
		// fetch and apply file
		c.remoteUpdate(address, msg)
	} else if op == shared.OpRemove {
		// remove is without file transfer, so directly apply
		c.applyUpdateWithMerge(msg)
	} else {
		c.warn("Unknown operation received, ignoring update message!")
	}
}

func (c *chaninterface) onRequestMessage(address string, msg shared.RequestMessage) {
	// c.log("Received request message!", msg.Request.String(), msg.Identification)
	// this means we need to send our selfpeer
	if msg.Request == shared.RePeer {
		// so build a bogus update message and send that
		peerPath := shared.TINZENITEDIR + "/" + shared.ORGDIR + "/" + shared.PEERSDIR + "/" + c.tin.selfpeer.Identification + shared.ENDING
		fullPath := shared.CreatePath(c.tin.model.Root, peerPath)
		obj, err := c.tin.model.GetInfo(fullPath)
		if err != nil {
			c.log("onRequestMessage:", err.Error())
			return
		}
		um := shared.CreateUpdateMessage(shared.OpCreate, *obj)
		c.tin.channel.Send(address, um.String())
		return
	}
	// get obj for path and directory
	obj, err := c.tin.model.GetInfoFrom(msg.Identification)
	if err != nil {
		c.log("Failed to locate object for", msg.Identification)
		return
	}
	if obj.Directory {
		c.warn("request is for directory, ignoring!")
		return
	}
	err = c.sendFile(address, c.tin.model.Root+"/"+obj.Path, msg.Identification, nil)
	if err != nil {
		c.log(err.Error())
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
			c.log("Failed to move file to temp: " + err.Error())
			return
		}
		// apply
		err = c.applyUpdateWithMerge(msg)
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
		c.log("ReModel:", err.Error())
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
sendFile sends the given file to the address. Path is where the file lies,
identification is what it will be named in transfer, and the function will be
called once the send was successful.
*/
func (c *chaninterface) sendFile(address, path, identification string, f channel.OnDone) error {
	// if no function is given we want to at least be notified if something went wrong, right?
	if f == nil {
		f = func(success bool) {
			if !success {
				log.Println("Send failed!", path)
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

/*
Log function that respects the AllowLogging flag.
*/
func (c *chaninterface) log(msg ...string) {
	if c.AllowLogging {
		toPrint := []string{"ChanInterface:"}
		toPrint = append(toPrint, msg...)
		log.Println(strings.Join(toPrint, " "))
	}
}

/*
Warn function.
*/
func (c *chaninterface) warn(msg ...string) {
	toPrint := []string{"ChanInterface:", "WARNING:"}
	toPrint = append(toPrint, msg...)
	log.Println(strings.Join(toPrint, " "))
}
