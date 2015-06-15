package core

import "errors"

// Errors for Context
var (
	ErrIsTinzenite  = errors.New("Already a Tinzenite directory!")
	ErrNotTinzenite = errors.New("Not a Tinzenite directory!")
)

// Context is the peer context that Core will work with.
type Context struct {
	Name    string
	Path    string
	channel *Channel
	Address string
	ToxData []byte
	Peers   []peer
}

/*
CreateContext creates a new Context attached to the given path.
*/
func CreateContext(name, dirpath string) (*Context, error) {
	// make sure it isn't already a Tinzenite directory
	if IsTinzenite(dirpath) {
		return nil, ErrIsTinzenite
	}
	var context Context
	var err error
	context.channel, err = CreateChannel(name, nil)
	if err != nil {
		return nil, err
	}
	context.Name = name
	context.Path = dirpath
	context.Address, _ = context.channel.Address()
	// and finally: store it (can we do this asynchroniously)
	err = context.Store()
	if err != nil {
		return nil, err
	}
	return &context, nil
}

/*
LoadContext loads a Context for the given path if it exists.
*/
func LoadContext(dirpath string) (*Context, error) {
	if !IsTinzenite(dirpath) {
		return nil, ErrNotTinzenite
	}
	var context *Context
	context, err := loadContext(dirpath)
	if err != nil {
		return nil, err
	}
	// init channel
	context.channel, err = CreateChannel(context.Name, context.ToxData)
	return context, err
}

/*
Store all important informaton.
*/
func (context *Context) Store() error {
	// get the current save data to store (otherwise we may lose data)
	saveData, err := context.channel.ToxData()
	if err != nil {
		return err
	}
	context.ToxData = saveData
	// and finally: store it (can we do this asynchroniously)
	return saveContext(context)
}
