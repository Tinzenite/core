package core

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/tinzenite/channel"
	"github.com/tinzenite/model"
	"github.com/tinzenite/shared"
)

/*
chaninterface implements the channel.Callbacks interface so that Tinzenite doesn't
export them unnecessarily.
*/
type chaninterface struct {
	tin          *Tinzenite              // reference back to Tinzenite
	inTransfers  map[string]transfer     // map of in transfers, referenced by the object id
	outTransfers map[string]bool         // map of out transfers, referenced by the object id
	active       map[string]bool         // stores running transfers
	challenges   map[string]int64        // store of SENT challenges. key is address, value is sent number
	connections  map[string]*shared.Peer // stores friend requests until they are accepted / denied
	recpath      string                  // shortcut to receiving dir
	temppath     string                  // shortcut to temp dir
}

func createChannelInterface(t *Tinzenite) *chaninterface {
	return &chaninterface{
		tin:          t,
		inTransfers:  make(map[string]transfer),
		outTransfers: make(map[string]bool),
		active:       make(map[string]bool),
		challenges:   make(map[string]int64),
		connections:  make(map[string]*shared.Peer),
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
	// try to read peer from message
	peer := &shared.Peer{}
	err := json.Unmarshal([]byte(message), peer)
	if err != nil {
		// TODO this is for debugging reasons: if non-peer conection attempt handle as trusted peer
		// FIXME this should result in an error
		peer, _ = shared.CreatePeer(message, address, true)
		log.Println("DEBUG: allowing non peer add of peer!")
	}
	// remember friend request
	c.connections[address] = peer
	// notify of incomming friend request (note that we do this async to not block this thread!)
	go c.tin.peerValidation(address, peer.Trusted)
}

/*
OnConnected is called whenever a peer comes online. Starts authentication process.
*/
func (c *chaninterface) OnConnected(address string) {
	c.log(address[:8], "came online!")
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
		// differentiate between encrypted and trusted behaviour
		if !trusted {
			c.onEncryptedMessage(address, v.Type, message)
		} else {
			// handle a trusted message
			c.onTrustedMessage(address, v.Type, message)
		}
		// return when done as the message has been worked
		return
	}
	// if unmarshal didn't work check for plain commands:
	switch message {
	default:
		// NOTE: Currently none implemented
		c.log("Received", message)
		c.tin.channel.Send(address, "ACK")
	}
}

// ----------------------- NORMAL FUNCTIONS ------------------------------------
// See also logic_*.go files for further functions

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
		nm := shared.CreateNotifyMessage(shared.NoRemoved, msg.Object.Name)
		c.tin.channel.Send(address, nm.JSON())
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
			return c.tin.channel.Send(address, rm.JSON())
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
	return c.tin.channel.Send(address, rm.JSON())
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
