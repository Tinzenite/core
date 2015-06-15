package core

type peer struct {
	Name      string
	Address   string
	Encrypted bool
}

func CreatePeer(name string, address string) *peer {
	return &peer{Name: name, Address: address, Encrypted: false}
}
