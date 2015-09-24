package core

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"

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
		// if LOCKED request model file to begin sync
		rm := shared.CreateRequestMessage(shared.OtModel, shared.IDMODEL)
		c.tin.channel.Send(address, rm.JSON())
	default:
		c.warn("Unknown lock action received:", msg.Action.String())
	}
}

func (c *chaninterface) onEncNotifyMessage(address string, msg shared.NotifyMessage) {
	switch msg.Notify {
	case shared.NoMissing:
		// remove transfer as no file will come
		delete(c.inTransfers, c.buildKey(address, msg.Identification))
		// if model --> create it
		if msg.Identification == shared.IDMODEL {
			log.Println("DEBUG: model is empty, skip directly to uploading!")
			err := c.doFullUpload(address)
			if err != nil {
				log.Println("DEBUG: ERROR:", err)
			}
			return
		}
		// if object --> error... maybe "reset" the encrypted peer?
		log.Println("DEBUG: something missing!", msg.Identification, msg.Notify)
	default:
		c.warn("Unknown notify type received:", msg.Notify.String())
	}
}

func (c *chaninterface) doFullUpload(address string) error {
	// write model to file
	model, err := ioutil.ReadFile(c.tin.Path + "/" + shared.STOREMODELDIR + "/" + shared.MODELJSON)
	if err != nil {
		return err
	}
	/*
		// TODO what nonce do we use? where do we put it?
		log.Println("DEBUG: WARNING: always using the same nonce for now, fix this!")
		// TODO write nonce PER FILE, append to encrypted data
		model, err = c.tin.auth.Encrypt(model, c.tin.auth.Nonce)
		if err != nil {
			return err
		}
	*/
	// write to temp file
	sendPath := c.tin.Path + "/" + shared.TINZENITEDIR + "/" + shared.SENDINGDIR + "/" + shared.IDMODEL
	err = ioutil.WriteFile(sendPath, model, shared.FILEPERMISSIONMODE)
	if err != nil {
		return err
	}
	// send model
	c.encSend(address, shared.IDMODEL, sendPath, shared.OtModel)
	return nil
}

/*
encSend handles uploading a file to the encrypted peer.
*/
func (c *chaninterface) encSend(address, identification, path string, ot shared.ObjectType) {
	pm := shared.CreatePushMessage(identification, ot)
	// TODO encrypt here?
	log.Println("TODO: where do we encrypt?")
	// send push notify
	_ = c.tin.channel.Send(address, pm.JSON())
	// send file
	_ = c.tin.channel.SendFile(address, path, identification, func(success bool) {
		if !success {
			c.log("Failed to upload file!", ot.String(), identification)
		}
		// remove sending temp file always
		err := os.Remove(path)
		if err != nil {
			c.warn("Failed to remove sending file!", err.Error())
		}
	})
	// done
}
