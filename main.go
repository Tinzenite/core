package core

import (
	"errors"
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
	peer     *Peer
	channel  *Channel
	allPeers []*Peer
	model    *objectinfo
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
	tinzenite.peer = peer
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
	// make sure model is ready
	tinzenite.createModel()
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
	// TODO remove from directory list
	return os.RemoveAll(path + "/" + TINZENITEDIR)
}

/*
SyncModel TODO
*/
func (tinzenite *Tinzenite) SyncModel() error {
	/*
		TODO
		- fetches model from other peers and syncs (this is for manual sync)
	*/
	// first ensure that local model is up to date
	err := tinzenite.updateModel()
	if err != nil {
		return err
	}
	// following concurrently?
	// iterate over all known peers
	// if online -> continue
	// if not init -> init
	// sync
	return nil
}

/*
Address of this Tinzenite peer.
*/
func (tinzenite *Tinzenite) Address() string {
	return tinzenite.peer.Address
}

/*
Close cleanly stores everything and shuts Tinzenite down.
*/
func (tinzenite *Tinzenite) Close() {
	tinzenite.channel.Close()
}

// RENAME + include in SyncModel?
func (tinzenite *Tinzenite) updateModel() error {
	if tinzenite.model == nil {
		return errors.New("Model has not yet been created!")
	}
	// TODO
	/*
				- updates from disk
				- How does this function know which context to use?
		        - add override for with path --> faster detection because not everything
		                              has to be rechecked
				- watch out that it doesn't bite itself with whatever method is used
						              to fetch models from online
	*/
	// use matcher on a per directory basis!
	return nil
}

func (tinzenite *Tinzenite) createModel() {
	rPath := relativePath{Root: tinzenite.Path}
	model, err := buildModel(rPath, false, tinzenite.allPeers)
	if err != nil {
		panic(err)
	}
	tinzenite.model = model
}

/*
write the tinzenite directory structure to disk.
*/
func (tinzenite *Tinzenite) write() error {
	// TODO
	/*
		Writes everything in the .tinzenite directory.
	*/
	root := tinzenite.Path + "/" + TINZENITEDIR
	// build directory structure
	err := makeDirectories(root,
		"org/peers", "temp", "removed")
	if err != nil {
		return err
	}
	// write all peers to files
	for _, peer := range tinzenite.allPeers {
		err = ioutil.WriteFile(root+"/org/peers/"+peer.identification, []byte(peer.Name), FILEPERMISSIONMODE)
		if err != nil {
			return err
		}
	}
	return nil
}

/*
callbackNewConnection is called when a new connection request comes in.
*/
func (tinzenite *Tinzenite) CallbackNewConnection(address, message string) {
	log.Printf("New connection from <%s> with message <%s>\n", address, message)
	err := tinzenite.channel.AcceptConnection(address)
	if err != nil {
		log.Println(err.Error())
	}
}

/*
callbackMessage is called when a message is received.
*/
func (tinzenite *Tinzenite) CallbackMessage(address, message string) {
	log.Printf("Message from <%s> with message <%s>\n", address, message)
	tinzenite.channel.Send(address, "ACK")
}

/*
storeGlobalConfig stores the path value into the user's home directory so that clients
can locate it.
*/
func (tinzenite *Tinzenite) storeGlobalConfig() error {
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
	_, err = file.WriteString(tinzenite.Path + "\n")
	if err != nil {
		return err
	}
	// ensure that the file is valid
	return PrettifyDirectoryList()
}
