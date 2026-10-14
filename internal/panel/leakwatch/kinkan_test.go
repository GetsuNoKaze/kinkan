package leakwatch

import (
	"mikan/internal/nodeapi"
	"mikan/internal/panel/geoip"
	"net/netip"
	"testing"
)

func TestSourcesCountNetworksNotAddressesOrUnknownSlots(t *testing.T) {
	online := map[string]nodeapi.Online{
		"user":     {Conns: 2, IPs: []string{"8.8.8.8", "8.8.4.4", "::ffff:8.8.8.8", "127.0.0.1", "bad"}},
		"device":   {Conns: 1, IPs: []string{"1.1.1.1"}},
		"inactive": {Conns: 0, IPs: []string{"9.9.9.9"}},
		"unknown":  {Conns: 1, IPs: []string{"9.9.9.9"}},
	}
	lookup := func(ip netip.Addr) geoip.Info {
		switch ip.String() {
		case "8.8.8.8", "8.8.4.4":
			return geoip.Info{ASN: "AS15169", Country: "US"}
		case "1.1.1.1":
			return geoip.Info{ASN: "AS13335", Country: "AU"}
		default:
			t.Fatalf("unexpected lookup: %s", ip)
			return geoip.Info{}
		}
	}
	out := Sources(online, map[string]int64{"user": 1, "device": 1, "inactive": 2}, lookup)
	if len(out) != 1 || len(out[1]) != 2 {
		t.Fatal(out)
	}
	unknown := Sources(online, map[string]int64{"user": 1}, func(netip.Addr) geoip.Info { return geoip.Info{} })
	if len(unknown) != 0 {
		t.Fatal("missing DB-IP counted as a network", unknown)
	}
}
