package proto

// TrustTunnel clients pool their TLS connections; mikan.client.max-connections is the most a
// client may keep to the inbound. The admin picks it per inbound, 1 when unset: before
// Citrus#54 the limit did not hold during a client's start and bursts of handshakes to one
// address and port got that port blocked on some Russian networks (12.10.2026).
const (
	kinkanMaxConnsKey     = "max-connections"
	KinkanTTConnsDefault  = 1
	KinkanTTConnsMaxLimit = 8 // mihomo's own default when the key is absent
)

// KinkanTTConnections is the client pool limit of a TrustTunnel template.
func KinkanTTConnections(t Template) int {
	ext, _ := t[extKey].(map[string]any)
	client, _ := ext["client"].(map[string]any)
	if n, ok := toInt(client[kinkanMaxConnsKey]); ok && n >= 1 && n <= KinkanTTConnsMaxLimit {
		return n
	}
	return KinkanTTConnsDefault
}

// KinkanSetTTConnections writes the client pool limit; the default removes the key.
func KinkanSetTTConnections(t Template, n int) error {
	if t.Type() != "trusttunnel" {
		return fail("config_key", extKey+".client."+kinkanMaxConnsKey)
	}
	if n < 1 || n > KinkanTTConnsMaxLimit {
		return fail("config_tt_connections", extKey+".client."+kinkanMaxConnsKey)
	}
	if n == KinkanTTConnsDefault {
		n = 0
	}
	setClient(t, kinkanMaxConnsKey, n)
	return nil
}

// kinkanValidateClient refuses a pool limit on another protocol or out of range.
func kinkanValidateClient(t Template) error {
	ext, _ := t[extKey].(map[string]any)
	client, _ := ext["client"].(map[string]any)
	raw, set := client[kinkanMaxConnsKey]
	if !set {
		return nil
	}
	field := extKey + ".client." + kinkanMaxConnsKey
	if t.Type() != "trusttunnel" {
		return fail("config_key", field)
	}
	if n, ok := toInt(raw); !ok || n < 1 || n > KinkanTTConnsMaxLimit {
		return fail("config_tt_connections", field)
	}
	return nil
}

func kinkanTrustTunnelClient(y map[string]any, t Template) {
	y["max-connections"] = KinkanTTConnections(t)
}
