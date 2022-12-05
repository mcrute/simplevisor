package supervise

import (
	"os"
	"os/user"
	"strconv"
)

func mustPipe() (*os.File, *os.File) {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	return r, w
}

func getUid(name string) (int, error) {
	u, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, err
	}
	return uid, nil
}

func getGid(name string) (int, error) {
	g, err := user.LookupGroup(name)
	if err != nil {
		return 0, err
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return 0, err
	}
	return gid, nil
}
