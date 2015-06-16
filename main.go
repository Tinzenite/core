package core

import (
	"errors"
	"log"
	"os/user"
)

/*
Errors that Tinzenite Core can return.
*/
// TODO move to const.go
var (
	ErrUnsupported  = errors.New("Feature currently unsupported!")
	ErrIsTinzenite  = errors.New("Already a Tinzenite directory!")
	ErrNotTinzenite = errors.New("Path is not valid Tinzenite directory!")
)

type Tinzenite struct {
}

func CreateTinzenite(path string, encrypted bool) (*Tinzenite, error) {
	// encrypted peer for now unsupported
	if encrypted {
		return nil, ErrUnsupported
	}
	if IsTinzenite(path) {
		return nil, ErrIsTinzenite
	}
	user, err := user.Current()
	if err != nil {
		return nil, err
	}
	tinzeniteConfigPath := user.HomeDir + "/.config/tinzenite" // + peer namehash (which also goes into the auth.json)
	log.Println(tinzeniteConfigPath)
	// TODO
	/*
	   - create context
	   - store everything for initial stuff
	   - ready connecting to rest of network
	*/
	return nil, nil
}

func LoadTinzenite(path string) (*Tinzenite, error) {
	if !IsTinzenite(path) {
		return nil, ErrNotTinzenite
	}
	// TODO
	/*
	   - load dir from given path (validate that path IS tinzenite first)
	*/
	return nil, nil
}

// RENAME
func (tinzenite *Tinzenite) UpdateModel() error {
	// TODO
	/*
				- updates from disk
				- How does this function know which context to use?
		        - add override for with path --> faster detection because not everything
		                              has to be rechecked
				- watch out that it doesn't bite itself with whatever method is used
						              to fetch models from online
	*/
	return nil
}

func (tinzenite *Tinzenite) SyncModel() error {
	// TODO
	/*
	   - fetches model from other peers and syncs (this is for manual sync)
	*/
	return nil
}
