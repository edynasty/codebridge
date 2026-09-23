package clientcred

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	key := Key("wss://manager.example/agent", "mac-1")
	if err := Save(path, key, "dev_secret"); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path, key)
	if err != nil {
		t.Fatal(err)
	}
	if got != "dev_secret" {
		t.Fatalf("got %q", got)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("credential file mode = %o, want 600", st.Mode().Perm())
		}
	}
}
