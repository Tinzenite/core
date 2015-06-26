package core

import (
	"io/ioutil"
	"log"
	"os"
	"os/user"
)

/*
Tinzenite is the struct on which all important operations should be called.
*/
type Tinzenite struct {
	Name     string
	Path     string
	Username string
	selfpeer *Peer
	channel  *Channel
	allPeers []*Peer
	model    *Model
}

/*
CreateTinzenite makes a directory a new Tinzenite directory. Will return error
if already so.
*/
func CreateTinzenite(dirname, dirpath, peername, username string, encrypted bool) (*Tinzenite, error) {
	// encrypted peer for now unsupported
	if encrypted {
		return nil, ErrUnsupported
	}
	if IsTinzenite(dirpath) {
		return nil, ErrIsTinzenite
	}
	// Build
	var tinzenite = &Tinzenite{
		Name:     dirname,
		Path:     dirpath,
		Username: username}
	// build channel
	channel, err := CreateChannel(peername, nil, tinzenite)
	if err != nil {
		return nil, err
	}
	tinzenite.channel = channel
	// build self peer
	address, err := channel.Address()
	if err != nil {
		return nil, err
	}
	peer, err := CreatePeer(peername, address)
	if err != nil {
		return nil, err
	}
	tinzenite.selfpeer = peer
	tinzenite.allPeers = []*Peer{peer}
	// save
	err = tinzenite.write()
	if err != nil {
		return nil, err
	}
	// save that this directory is now a tinzenite dir
	err = tinzenite.storeGlobalConfig()
	if err != nil {
		return nil, err
	}
	// build model (can block for long!)
	m, err := LoadModel(dirpath)
	if err != nil {
		return nil, err
	}
	tinzenite.model = m
	/*TODO later implement that model updates are sent to all online peers*/
	return tinzenite, nil
}

/*
LoadTinzenite will try to load the given directory path as a Tinzenite directory.
If not one it won't work: use CreateTinzenite to create a new peer.
*/
func LoadTinzenite(path string) (*Tinzenite, error) {
	if !IsTinzenite(path) {
		return nil, ErrNotTinzenite
	}
	/*
			TODO
		   - load dir from given path (validate that path IS tinzenite first)
	*/
	return nil, ErrUnsupported
}

/*
RemoveTinzenite directory. Specifically leaves all user files but removes all
Tinzenite specific items.
*/
func RemoveTinzenite(path string) error {
	if !IsTinzenite(path) {
		return ErrNotTinzenite
	}
	/* TODO remove from directory list*/
	return os.RemoveAll(path + "/" + TINZENITEDIR)
}

/*
SyncModel TODO

fetches model from other peers and syncs (this is for manual sync)
*/
func (t *Tinzenite) SyncModel() error {
	// first ensure that local model is up to date
	err := t.model.Update()
	if err != nil {
		return err
	}
	// iterate over all known peers
	for _, peer := range t.allPeers {
		if peer == t.selfpeer {
			continue
		}
		/*TODO - also make this concurrent?*/
		t.channel.Send(peer.Address, "Want update!")
		// if online -> continue
		// if not init -> init
		// sync
		/*TODO must store init state in allPeers too, also in future encryption*/
	}
	return nil
}

/*
Address of this Tinzenite peer.
*/
func (t *Tinzenite) Address() string {
	return t.selfpeer.Address
}

/*
Close cleanly stores everything and shuts Tinzenite down.
*/
func (t *Tinzenite) Close() {
	/*TODO should I really update again? Maybe just call store explicitely?*/
	t.model.Update()
	t.channel.Close()
}

/*
write the tinzenite directory structure to disk.
*/
func (t *Tinzenite) write() error {
	// TODO
	/*
		Writes everything in the .tinzenite directory.
	*/
	root := t.Path + "/" + TINZENITEDIR
	// build directory structure
	err := makeDirectories(root,
		"org/peers", "temp", "removed", "local")
	if err != nil {
		return err
	}
	// write all peers to files
	for _, peer := range t.allPeers {
		err = ioutil.WriteFile(root+"/org/peers/"+peer.identification, []byte(peer.Name), FILEPERMISSIONMODE)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *Tinzenite) Connect(address string) error {
	return t.channel.RequestConnection(address, t.selfpeer)
}

/*
CallbackNewConnection is called when a new connection request comes in.
*/
func (t *Tinzenite) CallbackNewConnection(address, message string) {
	log.Printf("New connection from <%s> with message <%s>\n", address, message)
	err := t.channel.AcceptConnection(address)
	if err != nil {
		log.Println(err.Error())
	}
	/*TODO actually this should be read from disk once the peer has synced... oO */
	/*
		tinzenite.allPeers = append(tinzenite.allPeers, &Peer{
			Name:     "Unknown",
			Address:  address,
			Protocol: Tox})
	*/
	// actually we just want to get type and confidence from the user here, and if everything
	// is okay we accept the connection --> then what? need to bootstrap him...
}

/*
CallbackMessage is called when a message is received.
*/
func (t *Tinzenite) CallbackMessage(address, message string) {
	log.Printf("Message from <%s> with message <%s>\n", address, message)
	t.channel.Send(address, "ACK")
	/*TODO switch if request or update*/
}

/*
storeGlobalConfig stores the path value into the user's home directory so that clients
can locate it.
*/
func (t *Tinzenite) storeGlobalConfig() error {
	// ready outside data
	user, err := user.Current()
	if err != nil {
		return err
	}
	path := user.HomeDir + "/.config/tinzenite"
	err = makeDirectory(path)
	if err != nil {
		return err
	}
	path += "/" + DIRECTORYLIST
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, FILEPERMISSIONMODE)
	if err != nil {
		return err
	}
	defer file.Close()
	// write path to file
	_, err = file.WriteString(t.Path + "\n")
	if err != nil {
		return err
	}
	// ensure that the file is valid
	return PrettifyDirectoryList()
}
