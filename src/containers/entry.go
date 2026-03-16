package containers

import (
	"fmt"
	"log"
	"sync"
	"time"

	"crypto/rand"

	"getaced.io/src/structs"
	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
)

var Client lxd.InstanceServer
var Snapshot *api.InstanceSnapshot
var IPPool *structs.IPPool

var Sessions = struct {
	sync.Mutex
	m map[string]*structs.SetupSession // token → session
}{m: make(map[string]*structs.SetupSession)}

func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

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

func GetSession(token string) *structs.SetupSession {
	Sessions.Lock()
	defer Sessions.Unlock()
	return Sessions.m[token]
}

func NewSession(userID uint) (string, error) {
	containerName := fmt.Sprintf("getaced-%d", userID)
	IPSuffix, err := IPPool.Acquire()
	token := generateToken()
	if err != nil {
		return "", fmt.Errorf("failed to acquire IP suffix for new session: %v", err)
	}

	// Clone from Snapshot
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

	// Strip MAC so LXD generates a unique one
	inst, etag, err := Client.GetInstance(containerName)
	if err != nil {
		return "", fmt.Errorf("failed to get instance config: %v", err)
	}
	delete(inst.Config, "volatile.eth0.hwaddr")
	updateOp, err := Client.UpdateInstance(containerName, inst.Writable(), etag)
	if err != nil {
		return "", fmt.Errorf("failed to clear MAC address: %v", err)
	}
	if err := updateOp.Wait(); err != nil {
		return "", fmt.Errorf("error waiting for MAC clear: %v", err)
	}

	// Start
	startOp, err := Client.UpdateInstanceState(containerName, api.InstanceStatePut{Action: "start"}, "")
	if err != nil {
		return "", fmt.Errorf("failed to start instance: %v", err)
	}
	if err := startOp.Wait(); err != nil {
		return "", fmt.Errorf("error waiting for start: %v", err)
	}

	execOp, err := Client.ExecInstance(containerName, api.InstanceExecPost{
		Command:     []string{"bash", "-c", fmt.Sprintf("cat > /etc/getaced.conf <<EOF\nIP_SUFFIX=%d\nEOF\n", IPSuffix)},
		WaitForWS:   true,
		Interactive: false,
	}, nil)
	if err != nil {
		return "", fmt.Errorf("failed to write IP suffix to /etc/getaced.conf in container: %v", err)
	}
	if err := execOp.Wait(); err != nil {
		return "", fmt.Errorf("error waiting for IP suffix write operation: %v", err)
	}

	// Put in setup mode
	execOp, err = Client.ExecInstance(containerName, api.InstanceExecPost{
		Command:     []string{"/root/start.sh", "setup"},
		WaitForWS:   false,
		Interactive: false,
	}, nil)
	if err != nil {
		return "", fmt.Errorf("failed to exec into container for setup mode: %w", err)
	}

	now := time.Now()
	Sessions.Lock()
	Sessions.m[token] = &structs.SetupSession{
		UserID:    userID,
		Container: containerName,
		IPSuffix:  IPSuffix,
		CreatedAt: now,
		ExpiresAt: now.Add(5 * time.Minute),
	}
	Sessions.Unlock()

	return token, nil
}

// CleanupExpiredSessions removes VNC sessions that have expired.
// The LXD container is NOT deleted - only the VNC access is revoked.
func CleanupExpiredSessions() {
	Sessions.Lock()
	defer Sessions.Unlock()

	now := time.Now()
	for token, session := range Sessions.m {
		if now.After(session.ExpiresAt) {
			log.Printf("session expired for user %d (container %s), revoking VNC access", session.UserID, session.Container)
			delete(Sessions.m, token)
		}
	}
}
