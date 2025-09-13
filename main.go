package main

import (
	"flag"
	"fmt"
	"os"

	"code.crute.us/mcrute/simplevisor/supervise"
)

func main() {
	mode := flag.String("mode", "parent", "mode in which to run simplevisor, internal use only")
	config := flag.String("config", "simplevisor.json", "config file location")
	noVault := flag.Bool("no-vault", false, "disable Vault integration entirely")
	discoverVault := flag.Bool("discover-vault", false, "use DNS SRV to discover Vault address")
	flag.Parse()

	switch *mode {
	case "parent":
		parent := &supervise.SupervisorParent{}
		parent.Main(*config, *noVault, *discoverVault)
	case "child":
		supervise.ChildMain()
	default:
		fmt.Println("Error starting supervisor, invalid mode passed.")
		os.Exit(1)
	}
}
