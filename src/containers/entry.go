package containers

import (
	"crypto/rand"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"getaced.io/src/structs"
	lxd "github.com/canonical/lxd/client"
	"github.com/canonical/lxd/shared/api"
)

var StartScript []byte

var Client lxd.InstanceServer
var IPPool *structs.IPPool

var Sessions = struct {
	sync.Mutex
	m map[string]*structs.SetupSession // token → session
}{m: make(map[string]*structs.SetupSession)}

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Printf("containers: crypto/rand error: %v", err)
	}
	return fmt.Sprintf("%x", b)
}

func Init() error {
	var err error
	Client, err = lxd.ConnectLXDUnix("/var/snap/lxd/common/lxd/unix.socket", nil)
	if err != nil {
		return fmt.Errorf("failed to connect to LXD: %v", err)
	}

	// Verify base container exists
	_, _, err = Client.GetInstance("getaced-base")
	if err != nil {
		return fmt.Errorf("failed to get base container 'getaced-base': %v", err)
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
			content, _, err := Client.GetInstanceFile(inst.Name, "/etc/getaced.conf")
			if err != nil {
				continue
			}
			buf := make([]byte, 256)
			n, err := content.Read(buf)
			if err != nil || n == 0 {
				continue
			}
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
	token := generateToken()

	ipSuffix, err := IPPool.Acquire()
	if err != nil {
		return "", fmt.Errorf("failed to acquire IP suffix for new session: %v", err)
	}

	// Release IP on any error
	success := false
	defer func() {
		if !success {
			IPPool.Release(ipSuffix)
		}
	}()

	// Copy base container (not snapshot)
	source, _, err := Client.GetInstance("getaced-base")
	if err != nil {
		return "", fmt.Errorf("failed to get base container: %v", err)
	}

	op, err := Client.CopyInstance(Client, *source, &lxd.InstanceCopyArgs{
		Name: containerName,
	})
	if err != nil {
		return "", fmt.Errorf("failed to copy container: %v", err)
	}
	if err := op.Wait(); err != nil {
		return "", fmt.Errorf("error waiting for copy operation: %v", err)
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

	// Write IP config
	err = Client.CreateInstanceFile(containerName, "/etc/getaced.conf", lxd.InstanceFileArgs{
		Content: strings.NewReader(fmt.Sprintf("IP_SUFFIX=%d\n", ipSuffix)),
		Type:    "file",
	})
	if err != nil {
		return "", fmt.Errorf("failed to write getaced.conf: %v", err)
	}

	// Inject start.sh from repo
	err = Client.CreateInstanceFile(containerName, "/root/start.sh", lxd.InstanceFileArgs{
		Content: strings.NewReader(string(StartScript)),
		Type:    "file",
		Mode:    0755,
	})
	if err != nil {
		return "", fmt.Errorf("failed to inject start.sh: %v", err)
	}

	// Put in setup mode
	_, err = Client.ExecInstance(containerName, api.InstanceExecPost{
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
		IPSuffix:  ipSuffix,
		CreatedAt: now,
		ExpiresAt: now.Add(5 * time.Minute),
	}
	Sessions.Unlock()

	success = true
	return token, nil
}

// CleanupExpiredSessions removes expired VNC sessions, stops the container, and releases the IP.
func CleanupExpiredSessions() {
	Sessions.Lock()
	var expired []*structs.SetupSession
	var expiredTokens []string
	now := time.Now()
	for token, session := range Sessions.m {
		if now.After(session.ExpiresAt) {
			expired = append(expired, session)
			expiredTokens = append(expiredTokens, token)
		}
	}
	for _, token := range expiredTokens {
		delete(Sessions.m, token)
	}
	Sessions.Unlock()

	// Stop containers and release IPs outside the lock
	for _, session := range expired {
		log.Printf("session expired for user %d (container %s), stopping container", session.UserID, session.Container)

		// Stop the container (kills Chrome, VNC, everything)
		op, err := Client.UpdateInstanceState(session.Container, api.InstanceStatePut{Action: "stop", Force: true}, "")
		if err != nil {
			log.Printf("containers: failed to stop expired session container %s: %v", session.Container, err)
		} else if err := op.Wait(); err != nil {
			log.Printf("containers: error waiting for stop of %s: %v", session.Container, err)
		}

		// Release IP back to the pool
		IPPool.Release(session.IPSuffix)
		log.Printf("containers: released IP suffix %d from expired session (user %d)", session.IPSuffix, session.UserID)
	}
}
