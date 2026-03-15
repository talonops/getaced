package handler

import (
	"fmt"
	"log"

	"getaced.io/src/containers"
	fws "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
)

// ChromeSession serves the noVNC HTML page
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
	   const rfb = new RFB(document.body, proto + location.host + '/v1/ws/%s/websockify');
	   rfb.scaleViewport = true;
	   rfb.resizeSession = true;
   </script>
   </body>
   </html>`, token)

	c.Set("Content-Type", "text/html")
	return c.SendString(html)
}

var wsUpgrader = fws.FastHTTPUpgrader{}

// WSProxy upgrades to websocket and proxies to the container's VNC
func WSProxy(c fiber.Ctx) error {
	token := c.Params("token")

	session := containers.GetSession(token)
	if session == nil {
		return c.Status(404).SendString("Session expired or invalid")
	}

	target := fmt.Sprintf("ws://10.246.29.%d:6080/websockify", session.IPSuffix)

	return wsUpgrader.Upgrade(c.RequestCtx(), func(clientConn *fws.Conn) {
		defer clientConn.Close()

		containerConn, _, err := fws.DefaultDialer.Dial(target, nil)
		if err != nil {
			log.Printf("failed to dial container VNC: %v", err)
			return
		}
		defer containerConn.Close()

		done := make(chan struct{})

		// container -> client
		go func() {
			defer close(done)
			for {
				mt, msg, err := containerConn.ReadMessage()
				if err != nil {
					return
				}
				if err := clientConn.WriteMessage(mt, msg); err != nil {
					return
				}
			}
		}()

		// client -> container
		for {
			mt, msg, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
			if err := containerConn.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	})
}
