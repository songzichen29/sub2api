package service

import "testing"

func TestValidateEndpoint_AcceptsHTTPAndHTTPS(t *testing.T) {
	cases := []string{
		"https://1.1.1.1",
		"http://1.1.1.1",
	}

	for _, endpoint := range cases {
		if err := validateEndpoint(endpoint); err != nil {
			t.Fatalf("expected endpoint %q to pass, got %v", endpoint, err)
		}
	}
}

func TestValidateEndpoint_RejectsInvalidScheme(t *testing.T) {
	if err := validateEndpoint("ftp://1.1.1.1"); err != ErrChannelMonitorEndpointScheme {
		t.Fatalf("expected invalid scheme error, got %v", err)
	}
}

func TestValidateEndpoint_RejectsPrivateHost(t *testing.T) {
	cases := []string{
		"https://127.0.0.1",
		"http://localhost",
	}

	for _, endpoint := range cases {
		if err := validateEndpoint(endpoint); err != ErrChannelMonitorEndpointPrivate {
			t.Fatalf("expected private host error for %q, got %v", endpoint, err)
		}
	}
}

func TestValidateEndpoint_RejectsPathQueryAndFragment(t *testing.T) {
	cases := []string{
		"https://1.1.1.1/v1",
		"https://1.1.1.1?foo=bar",
		"https://1.1.1.1#frag",
	}

	for _, endpoint := range cases {
		if err := validateEndpoint(endpoint); err != ErrChannelMonitorEndpointPath {
			t.Fatalf("expected path error for %q, got %v", endpoint, err)
		}
	}
}
