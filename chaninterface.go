package core

import (
	"encoding/json"
	"errors"
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
	tran, exists := c.inTransfers[identification]
	if !exists {
		c.log("Transfer not authorized for", identification, "!")
		return false, ""
	}
	if tran.active != address {
		c.log("Peer not authorized for transfer!")
		return false, ""
	}
	// check timeout
	if time.Since(tran.updated) > transferTimeout {
		// c.log("Transfer timed out!")
		delete(c.inTransfers, identification)
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
	// get tran
	tran, exists := c.inTransfers[identification]
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
	delete(c.inTransfers, identification)
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
	// the last index string is the identification, so we can delete the transfer
	delete(c.inTransfers, list[index])
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
	// NOTE the above go call works because the entire channel stuff runs in a
	// permament go routine – as long as it runs all child routines will be called! :D
}

/*
OnConnected is called whenever a peer comes online. Resets authentication
process if applicable to clean existing authentication from previous connects.
*/
func (c *chaninterface) OnConnected(address string) {
	c.log(address[:8], "came online!")
	// FIXME: resetting auth prevents trusted bootstrap.
	/*
		// we must only reset this if peer is trusted
		trusted, _ := c.tin.isPeerTrusted(address)
		if !trusted {
			return
		}
		// check if we can validly access it
		_, exists := c.tin.peers[address]
		if !exists {
			c.warn("Peer not found, can not reset authorization!")
			return
		}
		// otherwise fetch and set auth to false
		c.log("Resetting authorization for trusted peer.")
		c.tin.peers[address].SetAuthenticated(false)
	*/
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
		// all others are only allowed depending on auth status
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
	case "auth":
		log.Println("DEBUG: authorizing!")
		c.tin.peers[address].SetAuthenticated(true)
	case "deauth":
		log.Println("DEBUG: unauthorizing!")
		c.tin.peers[address].SetAuthenticated(false)
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
func (c *chaninterface) sendFile(address, path, identification string, f func(channel.State)) error {
	// we must wrap the function, even if none was given because we'll need to remove the outTransfers
	newFunction := func(status channel.State) {
		delete(c.outTransfers, identification)
		// remember to call the callback
		if f != nil {
			f(status)
		} else if status != channel.StSuccess {
			// if no function was given still alert that send failed
			log.Println("Transfer was not successful!", path)
		}
	}
	// if it already exists, don't restart a new one!
	_, exists := c.outTransfers[identification]
	if exists {
		// receiving side must restart if it so wants to, we'll just keep sending the original one
		return errors.New("out transfer already exists, will not resend")
	}
	// write that the transfer is happening
	c.outTransfers[identification] = true
	// now call with overwritten function
	return c.tin.channel.SendFile(address, path, identification, newFunction)
}

/*
requestFile requests the given request from the address and executes the function
when the transfer was successful. NOTE: only f may be nil.
*/
func (c *chaninterface) requestFile(address string, rm shared.RequestMessage, f onDone) error {
	// for all current transfers
	for identification, trans := range c.inTransfers {
		// skip if not wanted transfer
		if identification != rm.Identification {
			continue
		}
		// if transfer is being served from same address as the new request is sent
		if trans.active == address {
			// check for timeout for retransmit
			if time.Since(trans.updated) > transferTimeout {
				c.log("Retransmiting transfer due to timeout.")
				// update
				trans.updated = time.Now()
				c.inTransfers[identification] = trans
				// retransmit and done
				return c.tin.channel.Send(address, rm.JSON())
			}
			// if not yet time for retransmit ignore
			c.log("Ignoring multiple request for", identification, ".")
			// and return nil
			return nil
		}
		// if different address we shouldn't request it from somewhere else too
		c.log("Already fetching file", identification, "from other peer, ignoring!")
		/* TODO: add peer address to available peers to fetch update from for
		fall back purposes. NOTE that we should check if its for the same version
		of the object however - if not, replace it with more current version. */
		// and return nil
		return nil
	}
	// if transfer doesn't exist for identification, create it (and ONLY then create it)
	tran := transfer{
		updated: time.Now(),
		active:  address,
		done:    f}
	c.inTransfers[rm.Identification] = tran
	// request file from peer
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
determineObjectTypeBy is a small and ugly helper function used to flag when and
when not to decrypt in logic_encrypted.go.
*/
func (c *chaninterface) determineObjectTypeBy(path string) shared.ObjectType {
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
	return objectType
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
