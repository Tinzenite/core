package core

import (
	"log"
	"os"
)

/*TODO all model stuff here*/

/*
BuildModel does exactly that given any path.
*/
func buildModel(path *relativePath) error {
	stat, err := os.Lstat(path.FullPath())
	if err != nil {
		return err
	}
	log.Println("At: " + path.FullPath())
	if stat.IsDir() {
		log.Println("Directory: " + path.Subpath)
	} else {
		log.Println("File: " + path.Subpath)
	}
	return nil
}

/*
Model a path.
*/
func Model(path string) {
	// test relativePath
	p := &relativePath{Root: path}
	p.Down("/Music/")
	p.Up()
	p.Up()
	p.Down("Programming")
	p.Down("tinzenite")
	log.Println("Starting at " + p.FullPath())
	buildModel(p)
}
