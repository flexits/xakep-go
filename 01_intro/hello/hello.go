package main

import (
	"fmt"
	"os/user"

	"github.com/flexits/vt-enable"
	"github.com/goforj/godump"
)

func init() {
	vt.Enable()
}

func main() {
	var username string

	u, err := user.Current()
	if err != nil {
		username = "Χακερ"
	} else {
		username = u.Username
	}

	fmt.Printf("¡Hello %s!\n", username)

	godump.Dump(u)
}

/*
func main() {
	var (
		envKey   string
		username string
	)

	if runtime.GOOS == "windows" {
		envKey = "USERNAME"
	} else {
		envKey = "LOGNAME"
	}

	username, ok := os.LookupEnv(envKey)
	if !ok {
		username = "Χακερ"
	}

	fmt.Printf("¡Hello %s!\n", username)
}
*/
