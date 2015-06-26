package core

/*
UpdateMessage contains the relevant information for notifiying peers of updates.
*/
type UpdateMessage struct {
	Operation Operation
	Object    Objectinfo
}
