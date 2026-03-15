package containers

import (
	"fmt"
	"log"

	"getaced.io/src/structs"
	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
)

var Client lxd.InstanceServer
var Snapshot *api.InstanceSnapshot
var IPPool *structs.IPPool

func Init() error {
	var err error
	Client, err = lxd.ConnectLXDUnix("/var/snap/lxd/common/lxd/unix.socket", nil)
	if err != nil {
		return fmt.Errorf("failed to connect to LXD: %v", err)
	}

	Snapshot, _, err = Client.GetInstanceSnapshot("getaced-base", "ready")
	if err != nil {
		return fmt.Errorf("failed to get instance snapshot 'getaced-base/ready': %v", err)
	}

	IPPool = structs.NewIPPool()

	instances, err := Client.GetInstances(lxd.GetInstancesArgs{InstanceType: api.InstanceTypeContainer})
	if err != nil {
		return fmt.Errorf("failed to get instances: %v", err)
	}
	for _, inst := range instances {
		if inst.Name == "getaced-base" {
			continue
		}
		if inst.StatusCode == api.Running {
			// Read the IP suffix from container config
			content, _, err := Client.GetInstanceFile(inst.Name, "/etc/getaced.conf")
			if err != nil {
				continue
			}
			buf := make([]byte, 256)
			n, _ := content.Read(buf)
			var suffix int
			fmt.Sscanf(string(buf[:n]), "IP_SUFFIX=%d", &suffix)
			if suffix >= 101 && suffix <= 254 {
				IPPool.MarkUsed(suffix)
				log.Printf("recovered IP suffix %d from %s", suffix, inst.Name)
			}
		}
	}

	log.Println("containers initialized")
	return nil
}

func NewSession(userID string) (string, error) {

	containerName := fmt.Sprintf("getaced-%s", userID)
	IPSuffix := 123

	op, err := Client.CopyInstanceSnapshot(Client, "getaced-base", *Snapshot, &lxd.InstanceSnapshotCopyArgs{
		Name: containerName,
	})
	if err != nil {
		return "", fmt.Errorf("failed to clone from snapshot: %v", err)
	}

	err = op.Wait()
	if err != nil {
		return "", fmt.Errorf("error waiting for clone operation: %v", err)
	}

	execReq := api.InstanceExecPost{
		Command:     []string{"bash", "-c", fmt.Sprintf("cat > /etc/getaced.conf <<EOF\nIP_SUFFIX=%d\nEOF\n", IPSuffix)},
		WaitForWS:   true,
		Interactive: false,
	}

	execOp, err := Client.ExecInstance(containerName, execReq, nil)
	if err != nil {
		return "", fmt.Errorf("failed to exec into container: %v", err)
	}
	if err := execOp.Wait(); err != nil {
		return "", fmt.Errorf("error waiting for exec operation: %v", err)
	}

	// PORT 6080

	return containerName, nil

	/*

		startOp, err := Client.UpdateInstanceState(containerName, api.InstanceStatePut{Action: "start"}, "")
		if err != nil {
			log.Fatalf("failed to start instance: %v", err)
		}
		if err := startOp.Wait(); err != nil {
			log.Fatalf("error waiting for start: %v", err)
		}
	*/

	/*
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
