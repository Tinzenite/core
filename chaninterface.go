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
}

func createChannelInterface(t *Tinzenite) *chaninterface {
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
	// the message to apply once the file has been received
	success shared.UpdateMessage
}

/*
RequestFileTransfer is to be called by Tinzenite to authorize a file transfer
and to store what is to be done once it is successful. Handles multiplexing of
transfers as well. NOTE: Not a callback method.
*/
func (c *chaninterface) RequestFileTransfer(address string, um shared.UpdateMessage) {
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
OnAllowFile is the callback that checks whether the transfer is to be accepted or
not. Checks the address and identification of the object against c.transfers.
*/
func (c *chaninterface) OnAllowFile(address, identification string) (bool, string) {
	tran, exists := c.transfers[identification]
	if !exists {
		log.Println("Transfer not authorized!")
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
	tran, exists := c.transfers[identification]
	if !exists {
		log.Println("Transfer doesn't even exist anymore! Something bad went wrong...")
		// remove from transfers
		delete(c.transfers, identification)
		/*TODO remove any broken remaining temp files*/
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
	log.Printf("New connection from <%s> with message <%s>\n", address, message)
	err := c.tin.channel.AcceptConnection(address)
	if err != nil {
		log.Println(err.Error())
		return
	}
	/*TODO actually this should be read from disk once the peer has synced... oO
	Correction: read from message other peer info */
	newID, _ := shared.NewIdentifier()
	c.tin.allPeers = append(c.tin.allPeers, &shared.Peer{
		Identification: newID,   // must be read from message
		Name:           message, // must be read from message
		Address:        address,
		Protocol:       shared.CmTox})
	// actually we just want to get type and confidence from the user here, and if everything
	// is okay we accept the connection --> then what? need to bootstrap him...
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
			if op := msg.Operation; op == shared.OpCreate || op == shared.OpModify {
				// create & modify must first fetch file
				c.RequestFileTransfer(address, *msg)
			} else if op == shared.OpRemove {
				// remove is without file transfer, so directly apply
				c.applyUpdateWithMerge(*msg)
			} else {
				log.Println("Unknown operation received, ignoring update message!")
			}
		case shared.MsgRequest:
			// read request message
			msg := &shared.RequestMessage{}
			err := json.Unmarshal([]byte(message), msg)
			if err != nil {
				log.Println(err.Error())
				return
			}
			// get path
			/*TODO*/
			err = c.tin.channel.SendFile(address, c.tin.Path+"/Damned Society - Sunny on Sunday.mp3", msg.Identification)
			if err != nil {
				log.Println(err.Error())
				return
			}
		case shared.MsgModel:
			msg := &shared.ModelMessage{}
			err := json.Unmarshal([]byte(message), msg)
			if err != nil {
				log.Println(err.Error())
				return
			}
			// create updatemessage
			um, err := c.tin.model.SyncObject(&msg.Object)
			if err != nil {
				log.Println(err.Error())
				return
			}
			// if nil we don't need to do anything --> no update necessary
			if um == nil {
				log.Println("Nothing to do for object!")
				return
			}
			// send request for object
			c.RequestFileTransfer(address, *um)
		default:
			log.Printf("Unknown object sent: %s!\n", msgType)
		}
		// If it was JSON, we're done if we can't do anything with it
		return
	}
	// if unmarshal didn't work check for plain commands:
	switch message {
	case "model":
		model, err := c.tin.model.Read()
		/*TODO need to implement this better, model is too large for normal msg*/
		err = c.tin.send(address, model.String()[:1000])
		if err != nil {
			log.Println(err.Error())
		}
	case "auth":
		authbin, _ := json.Marshal(c.tin.auth)
		c.tin.channel.Send(address, string(authbin))
	case "create":
		// CREATE
		// messy but works: create file correctly, create objs, then move it to the correct temp location
		// first named create.txt to enable testing of create merge
		os.Create(c.tin.Path + "/create.txt")
		ioutil.WriteFile(c.tin.Path+"/create.txt", []byte("bonjour!"), shared.FILEPERMISSIONMODE)
		obj, _ := shared.CreateObjectInfo(c.tin.Path, "create.txt", "otheridhere")
		os.Rename(c.tin.Path+"/create.txt", c.tin.Path+"/"+shared.TINZENITEDIR+"/"+shared.TEMPDIR+"/"+obj.Identification)
		obj.Name = "test.txt"
		obj.Path = "test.txt"
		msg := shared.CreateUpdateMessage(shared.OpCreate, *obj)
		err := c.applyUpdateWithMerge(msg)
		if err != nil {
			log.Println("Create error: " + err.Error())
		}
	case "modify":
		// MODIFY
		obj, _ := shared.CreateObjectInfo(c.tin.Path, "test.txt", "otheridhere")
		orig, _ := c.tin.model.Objinfo[c.tin.Path+"/test.txt"]
		// id must be same
		obj.Identification = orig.Identification
		// version apply so that we can always "update" it
		obj.Version[c.tin.model.SelfID] = orig.Version[c.tin.model.SelfID]
		// if orig already has, increase further
		value, ok := orig.Version["otheridhere"]
		if ok {
			obj.Version["otheridhere"] = value
		}
		// add one new version
		obj.Version.Increase("otheridhere")
		um := shared.CreateUpdateMessage(shared.OpModify, *obj)
		err := c.tin.model.ApplyUpdateMessage(&um)
		if err != nil {
			log.Println(err.Error())
		}
	case "sendmodify":
		path := c.tin.Path + "/" + shared.TINZENITEDIR + "/" + shared.TEMPDIR
		orig, _ := c.tin.model.Objinfo[c.tin.Path+"/test.txt"]
		// write change to file in temp, simulating successful download
		ioutil.WriteFile(path+"/"+orig.Identification, []byte("send modify hello world!"), shared.FILEPERMISSIONMODE)
	case "testdir":
		// Test creation and removal of directory
		shared.MakeDirectory(c.tin.Path + "/dirtest")
		obj, _ := shared.CreateObjectInfo(c.tin.Path, "dirtest", "dirtestpeer")
		os.Remove(c.tin.Path + "/dirtest")
		um := shared.CreateUpdateMessage(shared.OpCreate, *obj)
		err := c.tin.model.ApplyUpdateMessage(&um)
		if err != nil {
			log.Println(err.Error())
		}
	case "conflict":
		// MODIFY that creates merge conflict
		path := c.tin.Path + "/" + shared.TINZENITEDIR + "/" + shared.TEMPDIR
		ioutil.WriteFile(c.tin.Path+"/merge.txt", []byte("written from conflict test"), shared.FILEPERMISSIONMODE)
		obj, _ := shared.CreateObjectInfo(c.tin.Path, "merge.txt", "otheridhere")
		os.Rename(c.tin.Path+"/merge.txt", path+"/"+obj.Identification)
		obj.Path = "test.txt"
		obj.Name = "test.txt"
		obj.Version[c.tin.model.SelfID] = -1
		obj.Version.Increase("otheridhere") // the remote change
		msg := shared.CreateUpdateMessage(shared.OpModify, *obj)
		err := c.applyUpdateWithMerge(msg)
		if err != nil {
			log.Println("Conflict error: " + err.Error())
		}
	case "delete":
		// DELETE
		obj, err := shared.CreateObjectInfo(c.tin.Path, "test.txt", "otheridhere")
		if err != nil {
			log.Println(err.Error())
			return
		}
		os.Remove(c.tin.Path + "/test.txt")
		um := shared.CreateUpdateMessage(shared.OpRemove, *obj)
		c.tin.model.ApplyUpdateMessage(&um)
		/*TODO implement remove merge conflict!*/
	case "showupdate":
		// helpful command that creates a model update message so that I can test it
		obj, _ := c.tin.model.GetInfo(shared.CreatePath(c.tin.Path, "test.txt"))
		mm := shared.CreateModelMessage(*obj)
		c.tin.send(address, mm.String())
	case "showrequest":
		obj, _ := c.tin.model.GetInfo(shared.CreatePath(c.tin.Path, "Damned Society - Sunny on Sunday.mp3"))
		rm := shared.CreateRequestMessage(shared.ReObject, obj.Identification)
		c.tin.send(address, rm.String())
	default:
		c.tin.channel.Send(address, "ACK")
	}
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
