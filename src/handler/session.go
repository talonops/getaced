package handler

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"getaced.io/src/containers"
	"github.com/gofiber/fiber/v3"
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

func WSProxy(c fiber.Ctx) error {

	log.Printf("WSProxy: Connection: %s", string(c.RequestCtx().Request.Header.Peek("Connection")))
	log.Printf("WSProxy: Upgrade: %s", string(c.RequestCtx().Request.Header.Peek("Upgrade")))
	log.Printf("WSProxy: WS-Key: %s", string(c.RequestCtx().Request.Header.Peek("Sec-WebSocket-Key")))
	log.Printf("WSProxy: WS-Protocol: %s", string(c.RequestCtx().Request.Header.Peek("Sec-WebSocket-Protocol")))

	token := c.Params("token")

	session := containers.GetSession(token)
	if session == nil {
		return c.Status(404).SendString("Session expired or invalid")
	}

	target := fmt.Sprintf("10.246.29.%d:6080", session.IPSuffix)
	log.Printf("WSProxy: proxying token=%s to %s", token, target)

	// Grab websocket headers from the browser's request
	reqCtx := c.RequestCtx()
	wsKey := string(reqCtx.Request.Header.Peek("Sec-WebSocket-Key"))
	wsVersion := string(reqCtx.Request.Header.Peek("Sec-WebSocket-Version"))
	wsProtocol := string(reqCtx.Request.Header.Peek("Sec-WebSocket-Protocol"))

	if wsKey == "" {
		return c.Status(400).SendString("Not a websocket request")
	}

	// Dial the container's websockify
	serverConn, err := net.Dial("tcp", target)
	if err != nil {
		log.Printf("WSProxy: failed to dial container: %v", err)
		return c.Status(502).SendString("Container not reachable")
	}

	// Build a websocket upgrade request using the browser's same key
	var upgradeReq strings.Builder
	upgradeReq.WriteString("GET /websockify HTTP/1.1\r\n")
	upgradeReq.WriteString(fmt.Sprintf("Host: %s\r\n", target))
	upgradeReq.WriteString("Upgrade: websocket\r\n")
	upgradeReq.WriteString("Connection: Upgrade\r\n")
	upgradeReq.WriteString(fmt.Sprintf("Sec-WebSocket-Key: %s\r\n", wsKey))
	upgradeReq.WriteString(fmt.Sprintf("Sec-WebSocket-Version: %s\r\n", wsVersion))
	if wsProtocol != "" {
		upgradeReq.WriteString(fmt.Sprintf("Sec-WebSocket-Protocol: %s\r\n", wsProtocol))
	}
	upgradeReq.WriteString("\r\n")

	// Send upgrade to container
	if _, err := serverConn.Write([]byte(upgradeReq.String())); err != nil {
		serverConn.Close()
		return c.Status(502).SendString("Failed to upgrade container")
	}

	// Read container's 101 response
	serverConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	serverBuf := bufio.NewReader(serverConn)
	resp, err := http.ReadResponse(serverBuf, nil)
	if err != nil || resp.StatusCode != 101 {
		serverConn.Close()
		log.Printf("WSProxy: container upgrade failed: %v (status: %d)", err, resp.StatusCode)
		return c.Status(502).SendString("Container upgrade failed")
	}
	serverConn.SetReadDeadline(time.Time{})

	// Rebuild the 101 response to forward to browser
	var clientResp bytes.Buffer
	clientResp.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	for k, vs := range resp.Header {
		for _, v := range vs {
			clientResp.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
		}
	}
	clientResp.WriteString("\r\n")

	// Hijack the browser connection — fasthttp hands us the raw TCP socket
	reqCtx.HijackSetNoResponse(true)
	reqCtx.Hijack(func(clientConn net.Conn) {
		defer clientConn.Close()
		defer serverConn.Close()

		// Send the 101 to the browser
		clientConn.Write(clientResp.Bytes())

		// If websockify already sent data with the 101, forward it
		if serverBuf.Buffered() > 0 {
			buffered, _ := serverBuf.Peek(serverBuf.Buffered())
			clientConn.Write(buffered)
		}

		// Pipe raw bytes both directions
		done := make(chan struct{})
		go func() {
			io.Copy(clientConn, serverConn)
			close(done)
		}()
		io.Copy(serverConn, clientConn)
		<-done
	})

	return nil
}
