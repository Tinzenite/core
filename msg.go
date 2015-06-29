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
