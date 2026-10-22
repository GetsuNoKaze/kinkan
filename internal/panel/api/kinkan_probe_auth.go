package api

import (
	"mikan/internal/nodeprobe"
	"mikan/internal/proto"
)

func kinkanProbeTransport(cfg *nodeprobe.Config, t proto.Template) {
	if udp, ok := t["udp"].(bool); ok {
		cfg.UDPNoise = udp
	}
	if n, ok := t["network"].(string); ok {
		cfg.Transport = n
	}
	// REALITY is itself an authentication layer: an ordinary TLS handshake with an
	// unknown key reaches its cover. Inner authentication cannot be reached that way.
	if _, reality := t["reality-config"]; reality {
		return
	}
	enabled, _ := t["tls"].(bool)
	if !enabled && (cfg.Protocol == "vless" || cfg.Protocol == "trojan") {
		cfg.PlainTCP = true
	}
}
