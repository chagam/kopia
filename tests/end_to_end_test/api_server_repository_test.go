package endtoend_test

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/kopia/kopia/internal/apiclient"
	"github.com/kopia/kopia/internal/serverapi"
	"github.com/kopia/kopia/internal/testlogging"
	"github.com/kopia/kopia/tests/testenv"
)

// foo@bar - password baz
var htpasswdFileContents = []byte("foo@bar:$2y$05$JWrExvBe5Knh0.AMLk5WHu.EzfOP.LhrqMIRf1YseZ/rulBjKqGJ.\n")

func TestAPIServerRepository(t *testing.T) {
	ctx := testlogging.Context(t)

	e := testenv.NewCLITest(t)
	defer e.Cleanup(t)
	defer e.RunAndExpectSuccess(t, "repo", "disconnect")

	// create one snapshot as foo@bar
	e.RunAndExpectSuccess(t, "repo", "create", "filesystem", "--path", e.RepoDir, "--override-username", "foo", "--override-hostname", "bar")
	e.RunAndExpectSuccess(t, "snapshot", "create", sharedTestDataDir1)

	e1 := testenv.NewCLITest(t)
	defer e1.Cleanup(t)
	defer e1.RunAndExpectSuccess(t, "repo", "disconnect")

	// create one snapshot as not-foo@bar
	e1.RunAndExpectSuccess(t, "repo", "connect", "filesystem", "--path", e.RepoDir, "--override-username", "not-foo", "--override-hostname", "bar")
	e1.RunAndExpectSuccess(t, "snapshot", "create", sharedTestDataDir1)

	htpasswordFile := filepath.Join(e.ConfigDir, "htpasswd.txt")
	ioutil.WriteFile(htpasswordFile, htpasswdFileContents, 0755)

	var sp serverParameters

	e.RunAndProcessStderr(t, sp.ProcessOutput,
		"server", "start",
		"--address=localhost:0",
		"--random-password",
		"--tls-generate-cert",
		"--auto-shutdown=60s",
		"--htpasswd-file", htpasswordFile,
	)
	t.Logf("detected server parameters %#v", sp)

	cli, err := apiclient.NewKopiaAPIClient(apiclient.Options{
		BaseURL:                             sp.baseURL,
		Username:                            "foo@bar",
		Password:                            "baz",
		TrustedServerCertificateFingerprint: sp.sha256Fingerprint,
		LogRequests:                         true,
	})
	if err != nil {
		t.Fatalf("unable to create API apiclient")
	}

	defer serverapi.Shutdown(ctx, cli) // nolint:errcheck

	waitUntilServerStarted(ctx, t, cli)

	e2 := testenv.NewCLITest(t)
	defer e2.Cleanup(t)
	defer e2.RunAndExpectSuccess(t, "repo", "disconnect")

	e2.RunAndExpectSuccess(t, "repo", "connect", "server",
		"--url", sp.baseURL,
		"--server-cert-fingerprint", sp.sha256Fingerprint,
		"--override-username", "foo",
		"--override-hostname", "bar",
		"--password", "baz",
	)

	// should see one snapshot
	snapshots := e2.ListSnapshotsAndExpectSuccess(t)
	if got, want := len(snapshots), 1; got != want {
		t.Errorf("invalid number of snapshots for foo@bar")
	}

	// create snapshot using remote repository client
	e2.RunAndExpectSuccess(t, "snapshot", "create", sharedTestDataDir2)

	// now should see two snapshots
	snapshots = e2.ListSnapshotsAndExpectSuccess(t)
	if got, want := len(snapshots), 2; got != want {
		t.Errorf("invalid number of snapshots for foo@bar")
	}
}
