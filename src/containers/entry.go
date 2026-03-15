package containers

import (
	"log"

	lxd "github.com/canonical/lxd/client"
)

var Client lxd.InstanceServer

func Init() error {
	var err error
	Client, err = lxd.ConnectLXDUnix("/var/snap/lxd/common/lxd/unix.socket", nil)
	if err != nil {
		return err
	}

	log.Println("lxd client initialized")
	return nil
}

func NewSession(userID string) {

	/*
		name := fmt.Sprintf("getaced-%s", userID)

		snapshot, _, err := Client.GetInstanceSnapshot("getaced-base", "ready")
		if err != nil {
			panic(err)
		}

		// Create the new instance from the snapshot using the LXD client
		op, err := Client.CopyInstanceSnapshot(containers.Client, "getaced-base", *snapshot, &lxd.InstanceSnapshotCopyArgs{
			Name: "getaced-1234",
		})
		if err != nil {
			log.Fatalf("failed to clone from snapshot: %v", err)
		}

		// Wait for the operation to complete
		err = op.Wait()
		if err != nil {
			log.Fatalf("error waiting for clone operation: %v", err)
		}

		startOp, err := Client.UpdateInstanceState("getaced-1234", api.InstanceStatePut{Action: "start"}, "")
		if err != nil {
			log.Fatalf("failed to start instance: %v", err)
		}
		if err := startOp.Wait(); err != nil {
			log.Fatalf("error waiting for start: %v", err)
		}

		execReq := api.InstanceExecPost{
			Command: []string{"bash", "-c", "echo 'IP_SUFFIX=101\nVNC_PASS=abc123' > /etc/getaced.conf"},
		}
		containers.Client.ExecInstance("getaced-1234", execReq, nil)
	*/
}
