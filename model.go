package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
)

type dirmodel struct {
	intype         dirint
	Identification string
	Path           string
	Shadow         bool
	Version        map[string]int
	Objects        []*objmodel
}

type filemodel struct {
	intype         fileint
	Identification string
	Path           string
	Shadow         bool
	Version        map[string]int
	Content        string
}

func buildModel(path relativePath, shadow bool, peers []*Peer) (objmodel, error) {
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
	model := dirmodel{Identification: id,
		Path:   path.Subpath,
		Shadow: shadow}
	versionDefault := map[string]int{}
	for _, peer := range peers {
		versionDefault[peer.identification] = 0
	}
	model.Version = versionDefault
	subStat, err := ioutil.ReadDir(path.FullPath())
	if err != nil {
		return nil, err
	}
	log.Println("Looking at " + path.FullPath())
	for _, stat := range subStat {
		var element objmodel
		subpath := path.Down(stat.Name())
		if stat.IsDir() {
			element, err = buildModel(*subpath, shadow, peers)
			if err != nil {
				return nil, err
			}
		} else {
			subid, err := newIdentifier()
			if err != nil {
				return nil, err
			}
			fm := filemodel{Identification: subid,
				Path:    subpath.Subpath,
				Shadow:  shadow,
				Content: "hash"}
			fm.Version = versionDefault
			element = fm
		}
		model.Objects = append(model.Objects, &element)
	}
	return model, nil
}

/*
func (dirm *dirmodel) walk(function func(object objmodel)) {
	for _, sub := range dirm.Objects {
		switch sub.(type) {
		case dirmodel:
		case fileint:
		}
	}
}
*/

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
