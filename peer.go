package core

import (
	"encoding/json"
	"io/ioutil"
)

/*
Peer is the communication representation of a Tinzenite peer.
*/
type Peer struct {
	Name           string
	Address        string
	Protocol       CommunicationMethod
	Encrypted      bool
	identification string
	initialized    bool
}

/*
CreatePeer creates a new object. For now always of type Tox.
*/
func CreatePeer(name string, address string) (*Peer, error) {
	id, err := newIdentifier()
	if err != nil {
		return nil, err
	}
	return &Peer{
		Name:           name,
		Address:        address,
		Protocol:       Tox,
		Encrypted:      false,
		identification: id}, nil
}

/*
JSON representation of peer.
*/
func (p *Peer) store(path string) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path+"/org/peers/"+p.identification, data, FILEPERMISSIONMODE)
}
