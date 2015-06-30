package core

type version map[string]int

/*
Increases the version for the given peer based on the already existing versions.
*/
func (v version) Increase(peerid string) {
	v[peerid] = v.Max() + 1
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
