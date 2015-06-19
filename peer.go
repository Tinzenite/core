package core

/*
Peer is the communication representation of a Tinzenite peer.
*/
type Peer struct {
	Name           string
	Address        string
	Protocol       CommunicationMethod
	Encrypted      bool
	identification string
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
