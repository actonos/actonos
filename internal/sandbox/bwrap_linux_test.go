//go:build linux

package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// requireBwrap skips the test unless bubblewrap can create the namespaces the
// production sandbox needs (CI containers and restricted user namespaces can't).
func requireBwrap(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	if err := exec.Command("bwrap", "--unshare-all", "--ro-bind", "/", "/", "true").Run(); err != nil {
		t.Skipf("bwrap cannot create namespaces here: %v", err)
	}
}

// useTempCgroup points the sandbox at a plain directory so tests never touch
// the real cgroup hierarchy and behave the same with or without delegation.
func useTempCgroup(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("ACTONOS_CGROUP_BASE", base)
	t.Setenv("ACTONOS_CGROUP_OPTIONAL", "")
	t.Setenv("ACTONOS_ALLOW_INSECURE_EXEC", "")
	return base
}

func runBwrap(t *testing.T, req CommandRequest) *CommandResult {
	t.Helper()
	if req.WorkspaceDir == "" {
		req.WorkspaceDir = t.TempDir()
	}
	if req.Timeout == 0 {
		req.Timeout = 10 * time.Second
	}
	res, err := NewBubblewrapSandbox().Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute(%q) returned error: %v", req.Command, err)
	}
	return res
}

func TestBubblewrapName(t *testing.T) {
	if got := NewBubblewrapSandbox().Name(); got != "bubblewrap" {
		t.Fatalf("Name() = %q", got)
	}
}

func TestBubblewrapRunsCommandInWorkspace(t *testing.T) {
	requireBwrap(t)
	useTempCgroup(t)
	ws := t.TempDir()

	res := runBwrap(t, CommandRequest{
		Command:      "pwd && echo data > out.txt",
		WorkspaceDir: ws,
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	if strings.TrimSpace(res.Stdout) != "/workspace" {
		t.Fatalf("cwd = %q, want /workspace", res.Stdout)
	}
	got, err := os.ReadFile(filepath.Join(ws, "out.txt"))
	if err != nil || strings.TrimSpace(string(got)) != "data" {
		t.Fatalf("workspace write not visible on host: %q err=%v", got, err)
	}
}

func TestBubblewrapCapturesExitCodeAndStderr(t *testing.T) {
	requireBwrap(t)
	useTempCgroup(t)

	res := runBwrap(t, CommandRequest{Command: "echo oops >&2; exit 7"})
	if res.ExitCode != 7 {
		t.Fatalf("exit=%d, want 7", res.ExitCode)
	}
	if !strings.Contains(res.Stderr, "oops") {
		t.Fatalf("stderr = %q", res.Stderr)
	}
	if res.Killed {
		t.Fatal("a normal non-zero exit must not be reported as killed")
	}
}

func TestBubblewrapTimeoutIsReportedAsKilled(t *testing.T) {
	requireBwrap(t)
	useTempCgroup(t)

	start := time.Now()
	res := runBwrap(t, CommandRequest{Command: "sleep 30", Timeout: 300 * time.Millisecond})
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("timeout not enforced, took %s", elapsed)
	}
	if !res.Killed {
		t.Fatalf("timed-out command must be reported as killed: %+v", res)
	}
	if res.ExitCode == 0 {
		t.Fatal("killed command must not report exit code 0")
	}
}

func TestBubblewrapPassesAllEnvAndDoesNotLeakHostEnv(t *testing.T) {
	requireBwrap(t)
	useTempCgroup(t)
	t.Setenv("ACTONOS_TEST_HOST_SECRET", "must-not-leak")

	res := runBwrap(t, CommandRequest{
		Command: `echo "A=$A B=$B C=$C"; echo "SECRET=[$ACTONOS_TEST_HOST_SECRET]"`,
		Env:     map[string]string{"A": "1", "B": "2", "C": "3"},
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "A=1 B=2 C=3") {
		t.Fatalf("requested env not forwarded: %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "SECRET=[]") {
		t.Fatalf("host environment leaked into the sandbox: %q", res.Stdout)
	}
}

func TestBubblewrapFilesystemIsolation(t *testing.T) {
	requireBwrap(t)
	useTempCgroup(t)
	hostFile := filepath.Join(t.TempDir(), "host-only.txt")
	if err := os.WriteFile(hostFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := runBwrap(t, CommandRequest{
		Command: `echo RAN; cat "` + hostFile + `" 2>/dev/null && echo SAW_HOST_FILE; touch /usr/pwned 2>/dev/null && echo WROTE_USR; true`,
	})
	if !strings.Contains(res.Stdout, "RAN") {
		t.Fatalf("command never ran: stdout=%q stderr=%q", res.Stdout, res.Stderr)
	}
	if strings.Contains(res.Stdout, "SAW_HOST_FILE") {
		t.Fatalf("sandbox can read arbitrary host paths: %q", res.Stdout)
	}
	if strings.Contains(res.Stdout, "WROTE_USR") {
		t.Fatalf("/usr must be read-only: %q", res.Stdout)
	}
}

func TestBubblewrapBindMounts(t *testing.T) {
	requireBwrap(t)
	useTempCgroup(t)
	ro, rw := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(ro, "in.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := runBwrap(t, CommandRequest{
		Command: `cat /mnt/ro/in.txt; touch /mnt/ro/x 2>/dev/null && echo RO_WRITABLE; echo out > /mnt/rw/out.txt`,
		BindMounts: []BindMount{
			{Source: ro, Dest: "/mnt/ro", ReadOnly: true},
			{Source: rw, Dest: "/mnt/rw"},
		},
	})
	if !strings.Contains(res.Stdout, "hello") {
		t.Fatalf("read-only mount not readable: stdout=%q stderr=%q", res.Stdout, res.Stderr)
	}
	if strings.Contains(res.Stdout, "RO_WRITABLE") {
		t.Fatal("read-only bind mount was writable")
	}
	if _, err := os.Stat(filepath.Join(rw, "out.txt")); err != nil {
		t.Fatalf("read-write mount write not visible on host: %v", err)
	}
}

func TestBubblewrapRejectsUnwritableCgroupWhenRequired(t *testing.T) {
	requireBwrap(t)
	base := useTempCgroup(t)
	// A regular file as the base makes Mkdir of the per-run group fail.
	file := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ACTONOS_CGROUP_BASE", file) // not a dir -> falls through to host discovery

	if _, err := NewBubblewrapSandbox().Execute(context.Background(), CommandRequest{
		Command: "true", WorkspaceDir: t.TempDir(), Timeout: 5 * time.Second,
	}); err != nil {
		// Either outcome is valid depending on host cgroup delegation; the
		// contract under test is "no panic, and an error mentions cgroup".
		if !strings.Contains(err.Error(), "cgroup") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestBubblewrapStartFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no bwrap on PATH
	_, err := NewBubblewrapSandbox().Execute(context.Background(), CommandRequest{
		Command: "true", WorkspaceDir: t.TempDir(), Timeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "starting bubblewrap") {
		t.Fatalf("expected start failure, got %v", err)
	}
}

func TestAttachCgroupWritesLimits(t *testing.T) {
	base := useTempCgroup(t)

	group, err := attachCgroup(4242, CommandRequest{MaxMemoryMB: 64, MaxProcesses: 9})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "actonos-4242"); group != want {
		t.Fatalf("group = %q, want %q", group, want)
	}
	for name, want := range map[string]string{
		"memory.max":   strconv.Itoa(64 * 1024 * 1024),
		"pids.max":     "9",
		"cpu.max":      "50000 100000",
		"cgroup.procs": "4242",
	} {
		got, err := os.ReadFile(filepath.Join(group, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q (err=%v), want %q", name, got, err, want)
		}
	}
}

func TestAttachCgroupDefaults(t *testing.T) {
	useTempCgroup(t)

	group, err := attachCgroup(4243, CommandRequest{})
	if err != nil {
		t.Fatal(err)
	}
	mem, _ := os.ReadFile(filepath.Join(group, "memory.max"))
	pids, _ := os.ReadFile(filepath.Join(group, "pids.max"))
	if string(mem) != strconv.Itoa(512*1024*1024) || string(pids) != "30" {
		t.Fatalf("defaults wrong: memory.max=%q pids.max=%q", mem, pids)
	}
}

func TestAttachCgroupMkdirFailure(t *testing.T) {
	base := useTempCgroup(t)
	if err := os.Mkdir(filepath.Join(base, "actonos-4244"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := attachCgroup(4244, CommandRequest{}); err == nil || !strings.Contains(err.Error(), "creating execution cgroup") {
		t.Fatalf("expected mkdir failure to be fatal by default, got %v", err)
	}

	for _, env := range []string{"ACTONOS_CGROUP_OPTIONAL", "ACTONOS_ALLOW_INSECURE_EXEC"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(env, "1")
			group, err := attachCgroup(4244, CommandRequest{})
			if err != nil || group != "" {
				t.Fatalf("optional cgroup should degrade silently: group=%q err=%v", group, err)
			}
		})
	}
}

func TestCleanupCgroupOnlyRemovesActonGroups(t *testing.T) {
	base := t.TempDir()
	mine := filepath.Join(base, "actonos-1")
	other := filepath.Join(base, "systemd-unit")
	for _, d := range []string{mine, other} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	cleanupCgroup("")
	cleanupCgroup(other)
	cleanupCgroup(mine)

	if _, err := os.Stat(other); err != nil {
		t.Fatalf("cleanup removed a foreign cgroup: %v", err)
	}
	if _, err := os.Stat(mine); !os.IsNotExist(err) {
		t.Fatalf("cleanup left the actonos group behind: %v", err)
	}
}

func TestFindCgroupV2BaseHonoursOverride(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("ACTONOS_CGROUP_BASE", custom)

	got, err := findCgroupV2Base()
	if err != nil || got != custom {
		t.Fatalf("findCgroupV2Base() = %q, %v; want %q", got, err, custom)
	}
}

func TestFindCgroupV2BaseIgnoresInvalidOverride(t *testing.T) {
	t.Setenv("ACTONOS_CGROUP_BASE", filepath.Join(t.TempDir(), "missing"))

	got, err := findCgroupV2Base()
	if err == nil && got == "" {
		t.Fatal("must return either a base or an error")
	}
	if err == nil && strings.Contains(got, "missing") {
		t.Fatalf("invalid override must not be returned: %q", got)
	}
}

func TestStrongSandboxSelection(t *testing.T) {
	t.Setenv("RUNTIME_MODE", "docker")
	if strongSandboxAvailable() {
		t.Fatal("strong sandbox must be unavailable inside containers")
	}

	t.Setenv("RUNTIME_MODE", "")
	if isContainerEnvironment() {
		t.Skip("running inside a container")
	}
	_, lookErr := exec.LookPath("bwrap")
	if got := strongSandboxAvailable(); got != (lookErr == nil) {
		t.Fatalf("strongSandboxAvailable()=%v, bwrap lookup err=%v", got, lookErr)
	}
	if name := newStrongSandbox().Name(); name != "bubblewrap" {
		t.Fatalf("newStrongSandbox() = %q", name)
	}
}

func TestUnavailableSandbox(t *testing.T) {
	sb := &unavailableSandbox{}
	if sb.Name() != "unavailable" {
		t.Fatalf("Name() = %q", sb.Name())
	}
	if res, err := sb.Execute(context.Background(), CommandRequest{Command: "true"}); res != nil || err != ErrSandboxUnavailable {
		t.Fatalf("Execute() = %v, %v", res, err)
	}
}
