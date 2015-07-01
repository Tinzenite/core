package core

import (
	"fmt"
	"log"
)

type version map[string]int

/*
Increases the version for the given peer based on the already existing versions.
*/
func (v version) Increase(selfid string) {
	/*TODO catch overflow on version increase!*/
	v[selfid] = v.Max() + 1
}

/*
Max version number from all listed peers.
*/
func (v version) Max() int {
	var max int
	for _, value := range v {
		if value >= max {
			max = value
		}
	}
	return max
}

func (v version) Valid(that version, selfid string) (version, bool) {
	if v.Max() > that.Max() {
		// other peer is missing updates!
		log.Println("Merge conflict! Modify is based on out of date file.")
		return v, false
	}
	if v[selfid] != that[selfid] {
		// this means local version was changed without the other peer realizing
		log.Println("Merge conflict! Local file has since changed.")
		return v, false
	}
	// otherwise we can update
	return that, true
}

/*
String representation of version.
*/
func (v version) String() string {
	var output string
	for key, value := range v {
		output += fmt.Sprintf("%s: %d\n", key, value)
	}
	return output
}
