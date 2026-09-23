package agentops

import "testing"

func TestSensitivePathPolicy(t *testing.T) {
	sensitive := []string{
		".env",
		".env.local",
		"app/.env.production",
		".git/config",
		".ssh/id_rsa",
		".aws/credentials",
		".kube/config",
		".docker/config.json",
		"private.pem",
		"tls/server.key",
		"cert/client.p12",
		"terraform.tfstate",
		"terraform.tfstate.backup",
		"prod.tfvars",
		"prod.tfvars.json",
	}
	for _, path := range sensitive {
		if !isSensitivePath(path) {
			t.Errorf("expected sensitive path: %q", path)
		}
	}

	safe := []string{
		".env.example",
		".env.sample",
		".env.template",
		".env.dist",
		"src/config.go",
		"README.md",
		"cert/public.crt",
	}
	for _, path := range safe {
		if isSensitivePath(path) {
			t.Errorf("unexpected sensitive path: %q", path)
		}
	}
}
