package core

type Peer struct {
	Name           string
	Address        string
	Protocol       CommunicationMethod
	Encrypted      bool
	identification string
}

func CreatePeer(name string, address string) (*Peer, error) {
	id, err := newIdentifier()
	if err != nil {
		return nil, err
	}
	return &Peer{
		Name:           name,
		Address:        address,
		Protocol:       tox,
		Encrypted:      false,
		identification: id}, nil
}
