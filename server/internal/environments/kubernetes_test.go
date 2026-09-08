package environments

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the actual command construction without requiring a Docker daemon
// or cluster. Only the exact deterministic tag has an image in this fixture.
func imageCleanupFixture(t *testing.T) (*Kubernetes, *Environment, string) {
	t.Helper()
	directory := t.TempDir()
	commands := map[string]string{
		"kubectl": `#!/bin/sh
exit 0
`,
		"kind": `#!/bin/sh
if [ "$1" = get ]; then printf 'test\n'; fi
`,
		"docker": `#!/bin/sh
printf '%s\n' "$*" >> "$STUB_COMMAND_LOG"
for argument do tag="$argument"; done
if [ "$1" = build ]; then
  case " $* " in
    *' --target test '*) printf 'test build failed\n' >&2; exit 1 ;;
  esac
  touch "$STUB_IMAGE_PRESENT"
  exit 0
fi
if [ "$STUB_DAEMON_UNAVAILABLE" = 1 ]; then
  printf 'Cannot connect to the Docker daemon\n' >&2
  exit 1
fi
case "$2" in
  inspect)
    if [ "$tag" != "$STUB_IMAGE_TAG" ] || [ ! -f "$STUB_IMAGE_PRESENT" ]; then
      printf 'No such image\n' >&2
      exit 1
    fi
    printf '{"aycorn.dev/installation":"%s","aycorn.dev/environment":"%s"}\n' "$STUB_INSTALLATION" "$STUB_ENVIRONMENT"
    ;;
  ls)
    if [ "$tag" = "reference=$STUB_IMAGE_TAG" ] && [ -f "$STUB_IMAGE_PRESENT" ]; then printf 'sha256:preview\n'; fi
    ;;
  rm)
    if [ "$STUB_REMOVE_FAILURE" = 1 ]; then printf 'image is in use\n' >&2; exit 1; fi
    rm "$STUB_IMAGE_PRESENT"
    ;;
  *) printf 'unexpected Docker command\n' >&2; exit 2 ;;
esac
`,
	}
	for name, script := range commands {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	k := &Kubernetes{Token: "installation"}
	e := &Environment{ID: 7, Settings: Defaults()}
	e.Settings.Context, e.Settings.KindCluster = "kind-test", "test"
	for key, value := range map[string]string{
		"STUB_COMMAND_LOG":        filepath.Join(directory, "commands"),
		"STUB_IMAGE_PRESENT":      filepath.Join(directory, "image"),
		"STUB_IMAGE_TAG":          k.imageTag(e),
		"STUB_INSTALLATION":       k.Token,
		"STUB_ENVIRONMENT":        "7",
		"STUB_DAEMON_UNAVAILABLE": "0",
		"STUB_REMOVE_FAILURE":     "0",
	} {
		t.Setenv(key, value)
	}
	return k, e, directory
}

func TestDestroyCleansPreviewImageAfterTestBuildFails(t *testing.T) {
	k, e, directory := imageCleanupFixture(t)
	e.Settings.PreviewTarget, e.Settings.TestTarget = "preview", "test"
	image, testImage, err := k.Build(context.Background(), e, directory, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "test build failed") {
		t.Fatalf("fixture did not fail the second build: %v", err)
	}
	if image != "" || testImage != "" || e.Image != "" || e.TestImage != "" {
		t.Fatal("fixture must represent an interrupted build without persisted image IDs")
	}
	if _, err = os.Stat(filepath.Join(directory, "image")); err != nil {
		t.Fatal("preview image was not built before the test target failed")
	}
	if err = k.Destroy(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(directory, "image")); !os.IsNotExist(err) {
		t.Fatalf("partially completed build leaked its preview image: %v", err)
	}
	// Repeated cleanup must safely ignore both tags now that they are absent.
	if err = k.Destroy(context.Background(), e); err != nil {
		t.Fatalf("repeated cleanup failed: %v", err)
	}
}

func TestDestroyImageFailuresRemainRetryable(t *testing.T) {
	for _, failure := range []struct{ name, key, value string }{
		{"wrong installation", "STUB_INSTALLATION", "other"},
		{"wrong environment", "STUB_ENVIRONMENT", "8"},
		{"daemon unavailable", "STUB_DAEMON_UNAVAILABLE", "1"},
		{"image in use", "STUB_REMOVE_FAILURE", "1"},
	} {
		t.Run(failure.name, func(t *testing.T) {
			k, e, directory := imageCleanupFixture(t)
			if err := os.WriteFile(filepath.Join(directory, "image"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			original := os.Getenv(failure.key)
			t.Setenv(failure.key, failure.value)
			if err := k.Destroy(context.Background(), e); err == nil {
				t.Fatal("cleanup reported success despite an image cleanup failure")
			}
			if _, err := os.Stat(filepath.Join(directory, "image")); err != nil {
				t.Fatal("cleanup removed an image despite the failed ownership or daemon check")
			}
			t.Setenv(failure.key, original)
			if err := k.Destroy(context.Background(), e); err != nil {
				t.Fatalf("cleanup did not recover when the failure was resolved: %v", err)
			}
		})
	}
}
