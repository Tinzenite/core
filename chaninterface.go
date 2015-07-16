package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
)

/*
chaninterface implements the channel.Callbacks interface so that Tinzenite doesn't
export them unnecessarily.
*/
type chaninterface struct {
	t *Tinzenite
}

/*
TODO finish implementing
*/
func (c *chaninterface) OnAllowFile(address, identification string) (bool, string) {
	// for now accept every transfer
	log.Printf("Allowing file <%s> from %s\n", identification, address)
	return true, c.t.Path + "/" + TINZENITEDIR + "/" + TEMPDIR + "/" + identification
}

/*
callbackFileReceived is for channel. It is called once the file has been successfully
received, thus initiates the actual local merging into the model.
*/
func (c *chaninterface) OnFileReceived(identification string) {
	log.Printf("File %s is now ready for model update!\n", identification)
	/*TODO check request if file is delta / must be decrypted before applying to model*/
}

/*
CallbackNewConnection is called when a new connection request comes in.
*/
func (c *chaninterface) OnNewConnection(address, message string) {
	log.Printf("New connection from <%s> with message <%s>\n", address, message)
	err := c.t.channel.AcceptConnection(address)
	if err != nil {
		log.Println(err.Error())
		return
	}
	/*TODO actually this should be read from disk once the peer has synced... oO
	Correction: read from message other peer info */
	newID, _ := newIdentifier()
	c.t.allPeers = append(c.t.allPeers, &Peer{
		Identification: newID,   // must be read from message
		Name:           message, // must be read from message
		Address:        address,
		Protocol:       CmTox})
	// actually we just want to get type and confidence from the user here, and if everything
	// is okay we accept the connection --> then what? need to bootstrap him...
}

/*
CallbackMessage is called when a message is received.
*/
func (c *chaninterface) OnMessage(address, message string) {
	// find out type of message
	v := &Message{}
	err := json.Unmarshal([]byte(message), v)
	if err == nil {
		switch msgType := v.Type; msgType {
		case MsgUpdate:
			msg := &UpdateMessage{}
			err := json.Unmarshal([]byte(message), msg)
			if err != nil {
				log.Println(err.Error())
				return
			}
			reqMsg := createRequestMessage(ReObject, msg.Object.Identification)
			c.t.channel.Send(address, reqMsg.String())
			/* TODO implement application of msg as wit manual command but will need to fetch file first...*/
		case MsgRequest:
			log.Println("Request received!")
			c.t.channel.Send(address, "Sending File (TODO)")
			/* TODO implement sending of file*/
		case MsgModel:
			msg := &ModelMessage{}
			err := json.Unmarshal([]byte(message), msg)
			if err != nil {
				log.Println(err.Error())
				return
			}
			// create updatemessage
			um, err := c.t.model.SyncObject(&msg.Object)
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
			c.t.send(address, um.String())
			/*TODO: allow file transfer, when done apply um*/
		default:
			log.Printf("Unknown object sent: %s!\n", msgType)
		}
		// If it was JSON, we're done if we can't do anything with it
		return
	}
	// if unmarshal didn't work check for plain commands:
	switch message {
	case "model":
		model, err := c.t.model.Read()
		/*TODO need to implement this better, model is too large for normal msg*/
		err = c.t.send(address, model.String()[:1000])
		if err != nil {
			log.Println(err.Error())
		}
	case "auth":
		authbin, _ := json.Marshal(c.t.auth)
		c.t.channel.Send(address, string(authbin))
	case "create":
		// CREATE
		// messy but works: create file correctly, create objs, then move it to the correct temp location
		// first named create.txt to enable testing of create merge
		os.Create(c.t.Path + "/create.txt")
		ioutil.WriteFile(c.t.Path+"/create.txt", []byte("bonjour!"), FILEPERMISSIONMODE)
		obj, _ := createObjectInfo(c.t.Path, "create.txt", "otheridhere")
		os.Rename(c.t.Path+"/create.txt", c.t.Path+"/"+TINZENITEDIR+"/"+TEMPDIR+"/"+obj.Identification)
		obj.Name = "test.txt"
		obj.Path = "test.txt"
		msg := &UpdateMessage{
			Operation: OpCreate,
			Object:    *obj}
		err := c.t.model.ApplyUpdateMessage(msg)
		if err == errConflict {
			err := c.t.merge(msg)
			if err != nil {
				log.Println("Merge: " + err.Error())
			}
		} else if err != nil {
			log.Println(err.Error())
		}
	case "modify":
		// MODIFY
		obj, _ := createObjectInfo(c.t.Path, "test.txt", "otheridhere")
		orig, _ := c.t.model.Objinfo[c.t.Path+"/test.txt"]
		// id must be same
		obj.Identification = orig.Identification
		// version apply so that we can always "update" it
		obj.Version[c.t.model.SelfID] = orig.Version[c.t.model.SelfID]
		// if orig already has, increase further
		value, ok := orig.Version["otheridhere"]
		if ok {
			obj.Version["otheridhere"] = value
		}
		// add one new version
		obj.Version.Increase("otheridhere")
		err := c.t.model.ApplyUpdateMessage(&UpdateMessage{
			Operation: OpModify,
			Object:    *obj})
		if err != nil {
			log.Println(err.Error())
		}
	case "sendmodify":
		path := c.t.Path + "/" + TINZENITEDIR + "/" + TEMPDIR
		orig, _ := c.t.model.Objinfo[c.t.Path+"/test.txt"]
		// write change to file in temp, simulating successful download
		ioutil.WriteFile(path+"/"+orig.Identification, []byte("send modify hello world!"), FILEPERMISSIONMODE)
	case "testdir":
		// Test creation and removal of directory
		makeDirectory(c.t.Path + "/dirtest")
		obj, _ := createObjectInfo(c.t.Path, "dirtest", "dirtestpeer")
		os.Remove(c.t.Path + "/dirtest")
		err := c.t.model.ApplyUpdateMessage(&UpdateMessage{
			Operation: OpCreate,
			Object:    *obj})
		if err != nil {
			log.Println(err.Error())
		}
	case "conflict":
		// MODIFY that creates merge conflict
		path := c.t.Path + "/" + TINZENITEDIR + "/" + TEMPDIR
		ioutil.WriteFile(c.t.Path+"/merge.txt", []byte("written from conflict test"), FILEPERMISSIONMODE)
		obj, _ := createObjectInfo(c.t.Path, "merge.txt", "otheridhere")
		os.Rename(c.t.Path+"/merge.txt", path+"/"+obj.Identification)
		obj.Path = "test.txt"
		obj.Name = "test.txt"
		obj.Version[c.t.model.SelfID] = -1
		obj.Version.Increase("otheridhere") // the remote change
		msg := &UpdateMessage{
			Operation: OpModify,
			Object:    *obj}
		err := c.t.model.ApplyUpdateMessage(msg)
		if err == errConflict {
			err := c.t.merge(msg)
			if err != nil {
				log.Println("Merge: " + err.Error())
			}
		} else {
			log.Println("WHY NO MERGE?!")
		}
	case "delete":
		// DELETE
		obj, err := createObjectInfo(c.t.Path, "test.txt", "otheridhere")
		if err != nil {
			log.Println(err.Error())
			return
		}
		os.Remove(c.t.Path + "/test.txt")
		c.t.model.ApplyUpdateMessage(&UpdateMessage{
			Operation: OpRemove,
			Object:    *obj})
		/*TODO implement remove merge conflict!*/
	case "show":
		// helpful command that creates a model update message so that I can test it
		obj, _ := c.t.model.getInfo(createPath(c.t.Path, "test.txt"))
		mm := createModelMessage(*obj)
		c.t.send(address, mm.String())
	default:
		c.t.channel.Send(address, "ACK")
	}
}
