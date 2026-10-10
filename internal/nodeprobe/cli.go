package nodeprobe

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"mikan/internal/ttprobe"
)

// Run retains the historical default (TrustTunnel) and adds protocol and manifest
// modes. A manifest contains probe endpoints only, never client credentials.
func Run(ctx context.Context, args []string, out, stderr io.Writer) error {
	f := flag.NewFlagSet("kinkan probe", flag.ContinueOnError)
	f.SetOutput(stderr)
	protocol := f.String("protocol", "trusttunnel", "inbound type: vless, vmess, trojan, anytls, trusttunnel, hysteria2, tuic, shadowquic, shadowsocks, snell, sudoku, mieru")
	ref := f.String("reference", "", "HTTPS cover on the same node (QUIC probes compare HTTP/3)")
	address := f.String("address", "", "pin target and cover connections to this IP; retain SNI and Host")
	timeout := f.Duration("timeout", 5*time.Second, "deadline per connection (100ms..30s)")
	jsonOut := f.Bool("json", false, "write JSON")
	manifest := f.String("inbounds", "", "JSON array of protocol, target, reference, address, obfuscated, alpn; no target argument")
	obfs := f.Bool("obfuscated", false, "port requires obfuscation (silence remains inconclusive)")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	var configs []Config
	if *manifest != "" {
		if f.NArg() != 0 {
			return errors.New("--inbounds does not accept a target argument")
		}
		file, err := os.Open(*manifest)
		if err != nil {
			return err
		}
		defer file.Close()
		dec := json.NewDecoder(io.LimitReader(file, 1<<20))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&configs); err != nil {
			return err
		}
		var extra any
		if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
			return errors.New("unexpected data after manifest")
		}
		if len(configs) == 0 || len(configs) > 64 {
			return errors.New("manifest must have 1..64 inbounds")
		}
	} else {
		if f.NArg() != 1 {
			return errors.New("usage: kinkan probe [--protocol TYPE] [--reference HTTPS_URL] [--address IP] [--json] HTTPS_URL; or --inbounds FILE")
		}
		// Existing invocations retain their report shape, output and exit semantics.
		if *protocol == "trusttunnel" && !*obfs {
			legacy := []string{"--timeout", timeout.String()}
			if *ref != "" {
				legacy = append(legacy, "--reference", *ref)
			}
			if *address != "" {
				legacy = append(legacy, "--address", *address)
			}
			if *jsonOut {
				legacy = append(legacy, "--json")
			}
			legacy = append(legacy, f.Arg(0))
			return ttprobe.Run(ctx, legacy, out, stderr)
		}
		configs = []Config{{Protocol: *protocol, Target: f.Arg(0), Reference: *ref, Address: *address, Obfuscated: *obfs}}
	}
	reports := make([]Report, 0, len(configs))
	failed, incomplete := 0, 0
	for _, cfg := range configs {
		cfg.Timeout = *timeout
		r, err := Scan(ctx, cfg)
		if err != nil {
			return err
		}
		reports = append(reports, r)
		if r.Verdict == "exposed" {
			failed++
		}
		if r.Incomplete || r.Verdict == "inconclusive" {
			incomplete++
		}
	}
	if *jsonOut {
		if err := json.NewEncoder(out).Encode(reports); err != nil {
			return err
		}
	} else {
		for _, r := range reports {
			if _, err := fmt.Fprintf(out, "%s %s: %s (incomplete=%t)\n", r.Protocol, r.Target, r.Verdict, r.Incomplete); err != nil {
				return err
			}
			for _, finding := range r.Findings {
				if _, err := fmt.Fprintf(out, "[%s] %s: %s\n", finding.Level, finding.Name, finding.Detail); err != nil {
					return err
				}
			}
		}
	}
	if failed != 0 || incomplete != 0 {
		return &ttprobe.CheckError{Failed: failed, Incomplete: incomplete}
	}
	return nil
}
