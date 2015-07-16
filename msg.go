package core

import "encoding/json"

/*
TODO: check if via Unmarshal Method we can output the enums as strings instead of
integer values. Should be written to enums.go, I guess?
*/

/*
Message is a base type for only reading out the operation to define the message
type.
*/
type Message struct {
	Type MsgType
}

/*
UpdateMessage contains the relevant information for notifiying peers of updates.
*/
type UpdateMessage struct {
	Type      MsgType
	Operation Operation
	Object    ObjectInfo
}

func createUpdateMessage(op Operation, obj ObjectInfo) UpdateMessage {
	return UpdateMessage{
		Type:      MsgUpdate,
		Operation: op,
		Object:    obj}
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
	Type    MsgType
	Request Request
	Object  string
}

func createRequestMessage(req Request, identification string) RequestMessage {
	return RequestMessage{
		Type:    MsgRequest,
		Request: req,
		Object:  identification}
}

func (rm *RequestMessage) String() string {
	data, _ := json.Marshal(rm)
	return string(data)
}

/*
ModelMessage is used to send a ObjectInfo, either completely or just a single
object.
*/
type ModelMessage struct {
	Type   MsgType
	Object ObjectInfo
}

func createModelMessage(object ObjectInfo) ModelMessage {
	return ModelMessage{
		Type:   MsgModel,
		Object: object}
}

func (mm *ModelMessage) String() string {
	data, _ := json.Marshal(mm)
	return string(data)
}
