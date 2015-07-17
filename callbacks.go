package core

/*
PeerValidation will be called if a peer tries to connect to this peer. The return
value will state whether the user accepts the connection.
*/
type PeerValidation func(address string, requestsTrust bool) bool

/*
RegisterPeerValidation registers a callback.
*/
func (t *Tinzenite) RegisterPeerValidation(f PeerValidation) {
	t.peerValidation = f
}
