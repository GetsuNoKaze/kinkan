package filters

import (
	"errors"
	"reflect"
	"testing"

	"mikan/internal/nodeapi"
)

func TestValidate(t *testing.T) {
	c := Config{
		Egress: Egress{Enabled: true,
			Ports:    []string{"6889-6881", "465", "465", "6881-6881", "1000 - 2000"},
			Networks: []string{"203.0.113.7/24", "198.51.100.9", "::ffff:198.51.100.9", "2001:DB8::1/32"},
			Domains:  []string{"*.Mail.Example", ".spam.example.", "mail.example"},
		},
		Ingress: Ingress{Enabled: true, Networks: []string{"10.0.0.1"}},
	}
	err := c.Validate()
	var p *Problem
	if !errors.As(err, &p) || p.List != "egress.ports" || p.Index != 0 {
		t.Fatalf("a reversed range: %v", err)
	}
	c.Egress.Ports[0] = "6881-6889"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	want := Egress{Enabled: true,
		Ports:    []string{"1000-2000", "465", "6881", "6881-6889"},
		Networks: []string{"198.51.100.9/32", "2001:db8::/32", "203.0.113.0/24"},
		Domains:  []string{"mail.example", "spam.example"},
	}
	if !reflect.DeepEqual(c.Egress, want) {
		t.Fatalf("canonical: %+v", c.Egress)
	}
	if c.Ingress.Networks[0] != "10.0.0.1/32" {
		t.Fatalf("ingress: %v", c.Ingress.Networks)
	}

	for list, bad := range map[string]Config{
		"egress.ports":     {Egress: Egress{Ports: []string{"0"}}},
		"egress.networks":  {Egress: Egress{Networks: []string{"300.1.1.1"}}},
		"egress.domains":   {Egress: Egress{Domains: []string{"bad,REJECT"}}},
		"ingress.networks": {Ingress: Ingress{Networks: []string{"fe80::1%eth0"}}},
	} {
		if err := bad.Validate(); !errors.As(err, &p) || p.List != list {
			t.Errorf("%s: %v", list, err)
		}
	}
	long := Config{Egress: Egress{Ports: make([]string, MaxItems+1)}}
	if err := long.Validate(); !errors.Is(err, ErrConfig) {
		t.Fatalf("too long: %v", err)
	}
}

func TestState(t *testing.T) {
	d := Default()
	if d.State() != nil {
		t.Fatal("off by default")
	}
	d.Egress.Enabled = true // the mail preset is on by default
	if got := d.State(); got == nil || !reflect.DeepEqual(got.Egress.Ports, []string{"2525", "465", "587"}) || len(got.Ingress.Networks) != 0 {
		t.Fatalf("mail: %+v", got)
	}
	d.Egress.Mail = false
	if d.State() != nil {
		t.Fatal("on with nothing to do is no filter")
	}
	d.Ingress = Ingress{Enabled: true, Allow: true, Networks: []string{"203.0.113.0/24"}}
	if got := d.State(); got == nil || !reflect.DeepEqual(got.Ingress, nodeapi.Ingress{Allow: true, Networks: []string{"203.0.113.0/24"}}) {
		t.Fatalf("ingress: %+v", got)
	}
	d.Ingress.Enabled = false
	if d.State() != nil {
		t.Fatal("ingress off")
	}
}
