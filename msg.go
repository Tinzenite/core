package core

import "encoding/json"

/*
UpdateMessage contains the relevant information for notifiying peers of updates.
*/
type UpdateMessage struct {
	Operation Operation
	Object    ObjectInfo
}

func (um *UpdateMessage) String() string {
	data, _ := json.Marshal(um)
	return string(data)
}

/*
RequestMessage is used to trigger the sending of messages or files from other
peers.
*/
type RequestMessage struct {
	Request Request
	Object  string
}

func (rm *RequestMessage) String() string {
	data, _ := json.Marshal(rm)
	return string(data)
}
