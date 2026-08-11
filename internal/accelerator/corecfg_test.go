package accelerator

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPatchCoreConfigPreservesUnknownKeysAndIsIdempotent(t *testing.T) {
	original := []byte(`// keep this comment
"Core"
{
	"CustomKey" "keep"
	"MinidumpUrl" "https://old.example/submit" // managed value
	"MinidumpPresubmit" "no"
}
`)
	patched, changes, err := patchCoreConfig(original, 9090, "token+value")
	if err != nil {
		t.Fatal(err)
	}
	text := string(patched)
	for _, want := range []string{
		`"CustomKey" "keep"`,
		"// keep this comment",
		`"MinidumpUrl" "http://127.0.0.1:9090/submit?token=token%2Bvalue"`,
		`"MinidumpSymbolUrl" "http://127.0.0.1:9090/symbols/submit?token=token%2Bvalue"`,
		`"MinidumpBinaryUrl" "http://127.0.0.1:9090/binary/submit?token=token%2Bvalue"`,
		`"MinidumpPresubmit" "yes"`,
		`"MinidumpSymbolUpload" "3"`,
		`"MinidumpBinaryUpload" "yes"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("patched config missing %q:\n%s", want, text)
		}
	}
	if len(changes) != 6 || changes["MinidumpUrl"].Previous != "https://old.example/submit" || changes["MinidumpSymbolUrl"].Present {
		t.Fatalf("changes=%#v", changes)
	}
	repeated, repeatedChanges, err := patchCoreConfig(patched, 9090, "token+value")
	if err != nil || !bytes.Equal(repeated, patched) || len(repeatedChanges) != 6 {
		t.Fatalf("repeat bytes equal=%v changes=%#v err=%v", bytes.Equal(repeated, patched), repeatedChanges, err)
	}
}

func TestRestoreCoreConfigRestoresMissingAndExistingManagedKeys(t *testing.T) {
	original := []byte(`"Core"
{
	"CustomKey" "keep"
	"MinidumpUrl" "https://old.example/submit"
}
`)
	patched, changes, err := patchCoreConfig(original, 8080, "secret")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := restoreCoreConfig(patched, changes)
	if err != nil || !bytes.Equal(restored, original) {
		t.Fatalf("restored=%q err=%v want=%q", restored, err, original)
	}
	modified := bytes.Replace(patched, []byte(`"MinidumpBinaryUpload" "yes"`), []byte(`"MinidumpBinaryUpload" "no"`), 1)
	if _, err := restoreCoreConfig(modified, changes); !errors.Is(err, ErrCoreConfigConflict) {
		t.Fatalf("modified core config error=%v", err)
	}
}

func TestPatchCoreConfigRejectsMalformedOrDuplicateManagedKeys(t *testing.T) {
	for _, original := range []string{
		`"Core" { "MinidumpUrl" "one" "MinidumpUrl" "two" }`,
		`"Core" { "MinidumpUrl" "unterminated"`,
		`"Other" { "MinidumpUrl" "nested" }`,
	} {
		if _, _, err := patchCoreConfig([]byte(original), 8080, "secret"); err == nil {
			t.Fatalf("accepted malformed/ambiguous config %q", original)
		}
	}
}
