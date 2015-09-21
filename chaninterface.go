package core

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	"github.com/tinzenite/shared"
)

/*
chaninterface implements the channel.Callbacks interface so that Tinzenite doesn't
export them unnecessarily.
*/
type chaninterface struct {
	tin          *Tinzenite          // reference back to Tinzenite
	inTransfers  map[string]transfer // map of in transfers, referenced by the object id
	outTransfers map[string]bool     // map of out transfers, referenced by the object id
	active       map[string]bool     // stores running transfers
	challenges   map[string]int64    // store of SENT challenges. key is address, value is sent number
	recpath      string              // shortcut to receiving dir
	temppath     string              // shortcut to temp dir
}

func createChannelInterface(t *Tinzenite) *chaninterface {
	return &chaninterface{
		tin:          t,
		inTransfers:  make(map[string]transfer),
		outTransfers: make(map[string]bool),
		active:       make(map[string]bool),
		challenges:   make(map[string]int64),
		recpath:      t.Path + "/" + shared.TINZENITEDIR + "/" + shared.RECEIVINGDIR,
		temppath:     t.Path + "/" + shared.TINZENITEDIR + "/" + shared.TEMPDIR}
}

// -------------------------CALLBACKS-------------------------------------------

/*
OnAllowFile is the callback that checks whether the transfer is to be accepted or
not. Checks the address and identification of the object against c.transfers.
*/
func (c *chaninterface) OnAllowFile(address, identification string) (bool, string) {
	key := c.buildKey(address, identification)
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
	if time.Since(tran.updated) > transferTimeout {
		// c.log("Transfer timed out!")
		delete(c.inTransfers, key)
		return false, ""
	}
	// here accept transfer
	// log.Printf("Allowing file <%s> from %s\n", identification, address)
	// add to active
	c.active[address] = true
	// name is address.identification to allow differentiating between same file from multiple peers
	return true, c.recpath + "/" + address + "." + identification
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
	key := c.buildKey(address, identification)
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
OnFileCanceled is called when a file transfer is cancelled. In that case we remove
the associated transfer.
*/
func (c *chaninterface) OnFileCanceled(address, path string) {
	// to build the key we require the last element after the last '.'
	list := strings.Split(path, ".")
	index := len(list) - 1
	// keep it sane
	if index < 0 || index >= len(list) {
		c.warn("OnFileCanceled: can not delete transfer: index out of range!")
		return
	}
	// the last index string is the identification, so we can build the key
	key := c.buildKey(address, list[index])
	delete(c.inTransfers, key)
}

/*
CallbackNewConnection is called when a bootstrap request comes in. This means
that the OTHER peer is bootstrapping: all we need to do here is save the other's
peer information and include it in the network if allowed.
*/
func (c *chaninterface) OnFriendRequest(address, message string) {
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
		// FIXME this should result in an error
		// this may happen for debug purposes etc
		peer = nil
		// TODO this is for debugging reasons: if non-peer conection attempt handle as trusted peer
		trusted = true
		log.Println("DEBUG: allowing non peer add of peer!")
	} else {
		// set trust value to what the other side WANTS to be
		trusted = peer.Trusted
	}
	// check if allowed
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
		// TODO remove this and fix it
		peer, _ = shared.CreatePeer(message, address, true)
		/*
			c.warn("No legal peer information could be read! Peer will be considered passive.")
			return
		*/
	}
	// ensure that address is correct by overwritting sent address with real one
	peer.Address = address
	// add peer to local list
	c.tin.peers[address] = peer
	// try store new peer to disk
	_ = c.tin.Store()
}

/*
OnConnected is called whenever a peer comes online. Starts authentication process.
*/
func (c *chaninterface) OnConnected(address string) {
	c.log(address[:8], "came online!")
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
		// special case for AuthenticationMessage
		if v.Type == shared.MsgChallenge {
			msg := &shared.AuthenticationMessage{}
			err := json.Unmarshal([]byte(message), msg)
			if err != nil {
				log.Println(err.Error())
				return
			}
			c.onAuthenticationMessage(address, *msg)
			// and done
			return
		}
		// all others are only allowed if authenticated
		trusted, err := c.tin.isPeerTrusted(address)
		if err != nil {
			c.log("OnMessage:", err.Error())
			return
		}
		// TODO differentiate between encrypted and trusted behaviour
		if !trusted {
			log.Println("TODO: implement for untrusted peers!")
			return
		}
		// switch behaviour for trusted messages
		switch msgType := v.Type; msgType {
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
		// If it was JSON, we're done if we can't do anything with it
		return
	}
	// if unmarshal didn't work check for plain commands:
	switch message {
	case "auth":
		// build and sent valid auth
		data := make([]byte, binary.MaxVarintLen64)
		_ = binary.PutVarint(data, int64(42))
		// get a nonce
		nonce := c.tin.auth.createNonce()
		// encrypt number with nonce
		encrypted, _ := c.tin.auth.Encrypt(data, nonce)
		reply := shared.CreateAuthenticationMessage(encrypted, nonce)
		c.tin.channel.Send(address, reply.JSON())
	default:
		// NOTE: Currently none implemented
		c.log("Received", message)
		c.tin.channel.Send(address, "ACK")
	}
}
