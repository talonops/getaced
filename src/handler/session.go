package handler

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"getaced.io/src/containers"
	"github.com/gofiber/fiber/v3"
	"github.com/gorilla/websocket"
)

func ChromeSession(c fiber.Ctx) error {
	token := c.Params("token")
	session := containers.GetSession(token)
	if session == nil {
		return c.Status(404).SendString("Session expired or invalid")
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
<title>GetAced - Chrome Setup</title>
<style>body{margin:0;overflow:hidden;background:#000}</style>
</head>
<body>
<script type="module">
    import RFB from '/novnc/core/rfb.js';
    const proto = location.protocol === 'https:' ? 'wss://' : 'ws://';
    const url = proto + location.host + '/v1/ws/%s/websockify';
    console.log('Connecting to:', url);
    const rfb = new RFB(document.body, url, { wsProtocols: ['binary'] });
    rfb.scaleViewport = true;
    rfb.resizeSession = true;
    rfb.addEventListener('connect', () => console.log('RFB connected'));
    rfb.addEventListener('disconnect', (e) => console.log('RFB disconnected', e.detail));
    rfb.addEventListener('securityfailure', (e) => console.log('RFB security failure', e.detail));
</script>
</body>
</html>`, token)

	c.Set("Content-Type", "text/html")
	return c.SendString(html)
}

var clientUpgrader = websocket.Upgrader{
	CheckOrigin:  func(r *http.Request) bool { return true },
	Subprotocols: []string{"binary"},
}

func WSProxyHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract token from /v1/ws/{token}/websockify
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	token := parts[3]

	session := containers.GetSession(token)
	if session == nil {
		http.Error(w, "Session expired or invalid", http.StatusNotFound)
		return
	}

	target := fmt.Sprintf("ws://10.246.29.%d:6080/websockify", session.IPSuffix)
	log.Printf("WSProxyHTTP: proxying token=%s to %s", token, target)

	// Upgrade client connection
	clientConn, err := clientUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WSProxyHTTP: client upgrade failed: %v", err)
		return
	}
	defer clientConn.Close()

	// Dial the container's websockify
	dialer := websocket.Dialer{
		Subprotocols: []string{"binary"},
	}
	serverConn, _, err := dialer.Dial(target, nil)
	if err != nil {
		log.Printf("WSProxyHTTP: failed to dial container: %v", err)
		return
	}
	defer serverConn.Close()

	// Bidirectional proxy
	done := make(chan struct{})

	// server -> client
	go func() {
		defer close(done)
		for {
			msgType, msg, err := serverConn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					log.Printf("WSProxyHTTP: server read error: %v", err)
				}
				return
			}
			if err := clientConn.WriteMessage(msgType, msg); err != nil {
				return
			}
		}
	}()

	// client -> server
	for {
		msgType, msg, err := clientConn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				log.Printf("WSProxyHTTP: client read error: %v", err)
			}
			break
		}
		if err := serverConn.WriteMessage(msgType, msg); err != nil {
			break
		}
	}

	<-done
}
