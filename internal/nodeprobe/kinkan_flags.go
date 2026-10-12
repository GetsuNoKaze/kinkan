package nodeprobe

import "flag"

type kinkanFlags struct {
	wrongKey, plain, udp *bool
	transport            *string
}

func kinkanProbeFlags(f *flag.FlagSet) kinkanFlags {
	return kinkanFlags{
		wrongKey:  f.Bool("wrong-key", false, "include structured wrong-key comparison (also for the default TrustTunnel report)"),
		plain:     f.Bool("plain-tcp", false, "target uses raw TCP without TLS; reference still uses HTTPS"),
		udp:       f.Bool("udp-noise", false, "also observe random UDP datagrams for noise transports; silence is inconclusive"),
		transport: f.String("transport", "", "inbound transport (tcp, ws, grpc, xhttp); unsupported framing is explicit"),
	}
}
func (f kinkanFlags) apply(cfg *Config) {
	if *f.udp {
		cfg.UDPNoise = true
	}
	if *f.plain {
		cfg.PlainTCP = true
	}
	if *f.transport != "" {
		cfg.Transport = *f.transport
	}
}

func (f kinkanFlags) legacy() bool { return !*f.wrongKey && !*f.plain && !*f.udp && *f.transport == "" }
