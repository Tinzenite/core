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

func defaultBuildModel(path relativePath, peer *Peer) (*objectinfo, error) {
	matcher, err := CreateMatcher(path.FullPath())
	if err != nil {
		return nil, err
	}
	return buildModel(path, false, []*Peer{peer}, *matcher)
}

func buildModel(path relativePath, shadow bool, peers []*Peer, matcher Matcher) (*objectinfo, error) {
	// ensure we're working on a directory
	stat, err := os.Lstat(path.FullPath())
	if err != nil {
		return nil, err
	}
	if !stat.IsDir() {
		return nil, errors.New(path.FullPath() + " is not a directory!")
	}
	// we'll need an id
	id, err := newIdentifier()
	if err != nil {
		return nil, err
	}
	this := objectinfo{
		directory:      true,
		Identification: id,
		Name:           path.LastElement(),
		Path:           path.Subpath(),
		Shadow:         shadow}
	versionDefault := map[string]int{}
	for _, peer := range peers {
		versionDefault[peer.identification] = 0
	}
	this.Version = versionDefault
	// load matcher for dir - take given one if none here
	if !matcher.Same(path.FullPath()) {
		thisMatcher, err := CreateMatcher(path.FullPath())
		if err != nil {
			return nil, err
		}
		if !thisMatcher.IsEmpty() {
			matcher = *thisMatcher
		}
	}
	// now work through all subfiles
	subStat, err := ioutil.ReadDir(path.FullPath())
	if err != nil {
		return nil, err
	}
	results := make(chan *objectinfo, 1)
	for _, stat := range subStat {
		go func(stat os.FileInfo, path relativePath) {
			subpath := path.Down(stat.Name())
			// check for things to ignore (NOTE: subpath because checking full path is kind of stupid, I think)
			if matcher.Ignore(subpath.Subpath()) {
				results <- nil
			}
			// recursion if dir
			if stat.IsDir() {
				element, err := buildModel(*subpath, shadow, peers, matcher)
				if err != nil {
					log.Println(err.Error())
					results <- nil
					return
				}
				results <- element
			} else {
				// each file gets new id
				subid, err := newIdentifier()
				if err != nil {
					log.Println(err.Error())
					results <- nil
					return
				}
				hash, err := contentHash(subpath.FullPath())
				if err != nil {
					log.Println(err.Error())
					results <- nil
					return
				}
				fm := objectinfo{
					directory:      false,
					Identification: subid,
					Name:           subpath.LastElement(),
					Path:           subpath.Subpath(),
					Shadow:         shadow,
					Content:        hash}
				fm.Version = versionDefault
				results <- &fm
			}
		}(stat, path)
	}
	for i := 0; i < len(subStat); i++ {
		result := <-results
		if result == nil {
			continue
		}
		this.Objects = append(this.Objects, result)
	}
	return &this, nil
}

/*
Model a path.
*/
func Model(path string) {
	p := relativePath{root: path}
	model, err := defaultBuildModel(p, &Peer{identification: "testpeer"})
	if err != nil {
		log.Println("Model returned with: " + err.Error())
	}
	// fmt.Printf("%+v\n", model)
	modelJSON, _ := json.MarshalIndent(model, "", "  ")
	fmt.Println(string(modelJSON))
}
