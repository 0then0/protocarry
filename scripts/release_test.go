package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReleaseChecksumAfterDownload(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("release packaging requires macOS or Linux")
	}
	script, err := filepath.Abs("release.sh")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	tools := filepath.Join(root, "tools")
	if err := os.Mkdir(tools, 0700); err != nil {
		t.Fatal(err)
	}
	// Stub compilation to isolate packaging and avoid recursively running go test.
	goStub := `#!/bin/sh
set -eu
case "$1" in
  env)
    case "$2" in
      GOOS) printf 'linux\n' ;;
      GOARCH) printf 'arm64\n' ;;
      *) exit 1 ;;
    esac ;;
  test|vet) exit 0 ;;
  build)
    shift
    while [ "$#" -gt 0 ]; do
      if [ "$1" = '-o' ]; then
        shift
        printf '#!/bin/sh\nprintf "ProtoCarry 0.1.0\\n"\n' > "$1"
        chmod +x "$1"
        exit 0
      fi
      shift
    done
    exit 1 ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(tools, "go"), []byte(goStub), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"README.md", "LICENSE"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("sh", script)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("release: %v\n%s", err, output)
	}
	download := t.TempDir()
	archive := "protocarry_0.1.0_linux_arm64.tar.gz"
	for _, name := range []string{archive, archive + ".sha256"} {
		data, err := os.ReadFile(filepath.Join(root, "bin", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(download, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	checker, err := exec.LookPath("sha256sum")
	args := []string{"-c", archive + ".sha256"}
	if err != nil {
		checker, err = exec.LookPath("shasum")
		args = []string{"-a", "256", "-c", archive + ".sha256"}
	}
	if err != nil {
		t.Fatal("release requires sha256sum or shasum")
	}
	cmd = exec.Command(checker, args...)
	cmd.Dir = download
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("downloaded archive checksum: %v\n%s", err, output)
	}
	// The checker must also reject a changed archive, not merely find the filename.
	if err := os.WriteFile(filepath.Join(download, archive), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(checker, args...)
	cmd.Dir = download
	if err := cmd.Run(); err == nil {
		t.Fatal("checksum accepted a corrupt archive")
	}
}
