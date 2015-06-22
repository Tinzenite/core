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
		log.Println("Directory: " + path.LastElement())
	} else {
		log.Println("File: " + path.LastElement())
	}
	return nil
}

/*
Model a path.
*/
func Model(path string) {
	// test relativePath
	p := &relativePath{Root: path}
	log.Println("Starting at " + p.FullPath())
	buildModel(p)
}
