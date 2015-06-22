package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

type objectinfo struct {
	directory      bool
	Identification string
	Name           string
	Path           string
	Shadow         bool
	Version        map[string]int
	Objects        []*objectinfo `json:",omitempty"`
	Content        string        `json:",omitempty"`
}

func buildModel(path relativePath, shadow bool, peers []*Peer) (*objectinfo, error) {
	stat, err := os.Lstat(path.FullPath())
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		return nil, errors.New(path.FullPath() + " is not a directory!")
	}
	id, err := newIdentifier()
	if err != nil {
		return nil, err
	}
	this := objectinfo{
		directory:      true,
		Identification: id,
		Name:           path.LastElement(),
		Path:           path.Subpath,
		Shadow:         shadow}
	versionDefault := map[string]int{}
	for _, peer := range peers {
		versionDefault[peer.identification] = 0
	}
	this.Version = versionDefault
	subStat, err := ioutil.ReadDir(path.FullPath())
	if err != nil {
		return nil, err
	}
	for _, stat := range subStat {
		var element *objectinfo
		subpath := path.Down(stat.Name())
		if stat.IsDir() {
			element, err = buildModel(*subpath, shadow, peers)
			if err != nil {
				return nil, err
			}
		} else {
			// each file gets new id
			subid, err := newIdentifier()
			if err != nil {
				return nil, err
			}
			fm := objectinfo{
				directory:      false,
				Identification: subid,
				Name:           subpath.LastElement(),
				Path:           subpath.Subpath,
				Shadow:         shadow,
				Content:        "hash"}
			fm.Version = versionDefault
			element = &fm
		}
		this.Objects = append(this.Objects, element)
	}
	return &this, nil
}

/*
Model a path.
*/
func Model(path string) {
	p := relativePath{Root: path}
	model, err := buildModel(p, false, []*Peer{&Peer{identification: "test"}})
	if err != nil {
		log.Println("Model returned with: " + err.Error())
	}
	fmt.Printf("%+v\n", model)
	modelJSON, _ := json.MarshalIndent(model, "", "  ")
	fmt.Print(string(modelJSON))
}
