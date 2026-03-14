package containers

import (
	"log"

	lxd "github.com/canonical/lxd/client"
)

var Client *lxd.InstanceServer

func Init() error {
	var err error
	var client lxd.InstanceServer
	client, err = lxd.ConnectLXDUnix("", nil)
	if err != nil {
		return err
	}
	Client = &client

	log.Println("lxd client initialized")
	return nil
}
