package core

import (
	"github.com/tinzenite/channel"
	"github.com/tinzenite/model"
	"github.com/tinzenite/shared"
)

/*
CreateTinzenite makes a directory a new Tinzenite directory. Will return error
if already so.
*/
func CreateTinzenite(dirname, dirpath, peername, username, password string) (*Tinzenite, error) {
	if shared.IsTinzenite(dirpath) {
		return nil, shared.ErrIsTinzenite
	}
	// flag whether we have to clean up after us
	var failed bool
	// make .tinzenite
	err := shared.MakeTinzeniteDir(dirpath)
	if err != nil {
		return nil, err
	}
	// if failed was set --> clean up by removing everything
	defer func() {
		if failed {
			shared.RemoveDotTinzenite(dirpath)
		}
	}()
	// get auth data
	auth, err := createAuthentication(dirpath, dirname, username, password)
	if err != nil {
		failed = true
		return nil, err
	}
	// Build
	tinzenite := &Tinzenite{
		Path: dirpath,
		auth: auth}
	// prepare chaninterface
	tinzenite.cInterface = createChannelInterface(tinzenite)
	// build channel
	channel, err := channel.Create(peername, nil, tinzenite.cInterface)
	if err != nil {
		failed = true
		return nil, err
	}
	tinzenite.channel = channel
	// build self peer
	address, err := channel.Address()
	if err != nil {
		failed = true
		return nil, err
	}
	peer, err := shared.CreatePeer(peername, address, true)
	if err != nil {
		failed = true
		return nil, err
	}
	// set own peer
	tinzenite.selfpeer = peer
	// prepare peer list
	tinzenite.peers = make(map[string]*shared.Peer)
	// add own peer to list of all peers
	tinzenite.peers[peer.Address] = peer
	// build model (can block for long!)
	m, err := model.Create(dirpath, peer.Identification, dirpath+"/"+shared.STOREMODELDIR)
	if err != nil {
		failed = true
		return nil, err
	}
	tinzenite.model = m
	// store initial copy
	err = tinzenite.Store()
	if err != nil {
		failed = true
		return nil, err
	}
	// save that this directory is now a tinzenite dir
	err = shared.WriteDirectoryList(tinzenite.Path)
	if err != nil {
		failed = true
		return nil, err
	}
	tinzenite.initialize()
	return tinzenite, nil
}

/*
LoadTinzenite will try to load the given directory path as a Tinzenite directory.
If not one it won't work: use CreateTinzenite to create a new peer.
*/
func LoadTinzenite(dirpath, password string) (*Tinzenite, error) {
	if !shared.IsTinzenite(dirpath) {
		return nil, shared.ErrNotTinzenite
	}
	t := &Tinzenite{Path: dirpath}
	// load auth
	auth, err := loadAuthenticationFrom(dirpath+"/"+shared.STOREAUTHDIR, password)
	if err != nil {
		return nil, err
	}
	t.auth = auth
	// load model
	model, err := model.LoadFrom(dirpath + "/" + shared.STOREMODELDIR)
	if err != nil {
		return nil, err
	}
	t.model = model
	// load peer list
	peers, err := shared.LoadPeers(dirpath)
	if err != nil {
		return nil, err
	}
	t.peers = peers
	// load tox dump
	selfToxDump, err := shared.LoadToxDumpFrom(dirpath + "/" + shared.STORETOXDUMPDIR)
	if err != nil {
		return nil, err
	}
	t.selfpeer = selfToxDump.SelfPeer
	// prepare chaninterface
	t.cInterface = createChannelInterface(t)
	// prepare channel
	channel, err := channel.Create(t.selfpeer.Name, selfToxDump.ToxData, t.cInterface)
	if err != nil {
		return nil, err
	}
	t.channel = channel
	t.initialize()
	// empty temp folder to remove orphaned files (ignore error because we don't care if it works)
	_ = shared.RemoveDirContents(t.Path + "/" + shared.TINZENITEDIR + "/" + shared.TEMPDIR)
	return t, nil
}
