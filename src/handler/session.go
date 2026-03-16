package handler

import (
	"fmt"
	"strings"

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
    const rfb = new RFB(document.body, url, { shared: true, credentials: { password: '' } });
    rfb.scaleViewport = true;
    rfb.resizeSession = true;
    rfb.addEventListener('connect', () => console.log('RFB connected'));
    rfb.addEventListener('disconnect', (e) => console.log('RFB disconnected', e.detail));
    rfb.addEventListener('credentialsrequired', () => console.log('RFB credentials required'));
    rfb.addEventListener('securityfailure', (e) => console.log('RFB security failure', e.detail));
</script>
</body>
</html>`, token)

	c.Set("Content-Type", "text/html")
	return c.SendString(html)
}

// ResolveWS is called by nginx auth_request to resolve a session token to a container IP.
// Returns 200 with X-Target header on success, 401 on invalid token.
func ResolveWS(c fiber.Ctx) error {
	// nginx passes the original URI in X-Original-URI header
	// e.g. /v1/ws/{token}/websockify
	uri := c.Get("X-Original-URI")
	parts := strings.Split(uri, "/")
	if len(parts) < 4 {
		return c.SendStatus(401)
	}
	token := parts[3]

	session := containers.GetSession(token)
	if session == nil {
		return c.SendStatus(401)
	}

	c.Set("X-Target", fmt.Sprintf("10.246.29.%d:6080", session.IPSuffix))
	return c.SendStatus(200)
}
