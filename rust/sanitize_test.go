package rust

import (
	"fmt"
	"strings"
	"testing"

	"android/soong/android"
)

type MemtagNoteType int

const (
	None MemtagNoteType = iota + 1
	Sync
	Async
)

func (t MemtagNoteType) str() string {
	switch t {
	case None:
		return "none"
	case Sync:
		return "sync"
	case Async:
		return "async"
	default:
		panic("type_note_invalid")
	}
}

func checkHasMemtagNote(t *testing.T, m android.TestingModule, expected MemtagNoteType) {
	t.Helper()
	note_async := "note_memtag_heap_async"
	note_sync := "note_memtag_heap_sync"

	found := None
	implicits := m.Rule("rustc").Implicits
	for _, lib := range implicits {
		if strings.Contains(lib.Rel(), note_async) {
			found = Async
			break
		} else if strings.Contains(lib.Rel(), note_sync) {
			found = Sync
			break
		}
	}

	if found != expected {
		t.Errorf("Wrong Memtag note in target %q: found %q, expected %q", m.Module().(*Module).Name(), found.str(), expected.str())
	}
}

var prepareForTestWithMemtagHeap = android.GroupFixturePreparers(
	android.FixtureModifyMockFS(func(fs android.MockFS) {
		templateBp := `
		rust_test {
			name: "unset_test_%[1]s",
			srcs: ["foo.rs"],
		}

		rust_test {
			name: "no_memtag_test_%[1]s",
			srcs: ["foo.rs"],
			sanitize: { memtag_heap: false },
		}

		rust_test {
			name: "set_memtag_test_%[1]s",
			srcs: ["foo.rs"],
			sanitize: { memtag_heap: true },
		}

		rust_test {
			name: "set_memtag_set_async_test_%[1]s",
			srcs: ["foo.rs"],
			sanitize: { memtag_heap: true, diag: { memtag_heap: false }  },
		}

		rust_test {
			name: "set_memtag_set_sync_test_%[1]s",
			srcs: ["foo.rs"],
			sanitize: { memtag_heap: true, diag: { memtag_heap: true }  },
		}

		rust_test {
			name: "unset_memtag_set_sync_test_%[1]s",
			srcs: ["foo.rs"],
			sanitize: { diag: { memtag_heap: true }  },
		}

		rust_binary {
			name: "unset_binary_%[1]s",
			srcs: ["foo.rs"],
		}

		rust_binary {
			name: "no_memtag_binary_%[1]s",
			srcs: ["foo.rs"],
			sanitize: { memtag_heap: false },
		}

		rust_binary {
			name: "set_memtag_binary_%[1]s",
			srcs: ["foo.rs"],
			sanitize: { memtag_heap: true },
		}

		rust_binary {
			name: "set_memtag_set_async_binary_%[1]s",
			srcs: ["foo.rs"],
			sanitize: { memtag_heap: true, diag: { memtag_heap: false }  },
		}

		rust_binary {
			name: "set_memtag_set_sync_binary_%[1]s",
			srcs: ["foo.rs"],
			sanitize: { memtag_heap: true, diag: { memtag_heap: true }  },
		}

		rust_binary {
			name: "unset_memtag_set_sync_binary_%[1]s",
			srcs: ["foo.rs"],
			sanitize: { diag: { memtag_heap: true }  },
		}
		`
		subdirNoOverrideBp := fmt.Sprintf(templateBp, "no_override")
		subdirOverrideDefaultDisableBp := fmt.Sprintf(templateBp, "override_default_disable")
		subdirSyncBp := fmt.Sprintf(templateBp, "override_default_sync")
		subdirAsyncBp := fmt.Sprintf(templateBp, "override_default_async")

		fs.Merge(android.MockFS{
			"subdir_no_override/Android.bp":              []byte(subdirNoOverrideBp),
			"subdir_override_default_disable/Android.bp": []byte(subdirOverrideDefaultDisableBp),
			"subdir_sync/Android.bp":                     []byte(subdirSyncBp),
			"subdir_async/Android.bp":                    []byte(subdirAsyncBp),
		})
	}),
	android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
		variables.MemtagHeapExcludePaths = []string{"subdir_override_default_disable"}
		// "subdir_override_default_disable" is covered by both include and override_default_disable paths. override_default_disable wins.
		variables.MemtagHeapSyncIncludePaths = []string{"subdir_sync", "subdir_override_default_disable"}
		variables.MemtagHeapAsyncIncludePaths = []string{"subdir_async", "subdir_override_default_disable"}
	}),
)

func TestSanitizeMemtagHeap(t *testing.T) {
	variant := "android_arm64_armv8-a"

	result := android.GroupFixturePreparers(
		prepareForRustTest,
		prepareForTestWithMemtagHeap,
	).RunTest(t)
	ctx := result.TestContext

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_sync", variant), Sync)
}

func TestSanitizeMemtagHeapWithSanitizeDevice(t *testing.T) {
	variant := "android_arm64_armv8-a"

	result := android.GroupFixturePreparers(
		prepareForRustTest,
		prepareForTestWithMemtagHeap,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.SanitizeDevice = []string{"memtag_heap"}
		}),
	).RunTest(t)
	ctx := result.TestContext

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_sync", variant), Sync)

	// should sanitize: { diag: { memtag: true } } result in Sync instead of None here?
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_async", variant), Sync)
	// should sanitize: { diag: { memtag: true } } result in Sync instead of None here?
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_sync", variant), Sync)
}

func TestSanitizeMemtagHeapWithSanitizeDeviceDiag(t *testing.T) {
	variant := "android_arm64_armv8-a"

	result := android.GroupFixturePreparers(
		prepareForRustTest,
		prepareForTestWithMemtagHeap,
		android.FixtureModifyProductVariables(func(variables android.FixtureProductVariables) {
			variables.SanitizeDevice = []string{"memtag_heap"}
			variables.SanitizeDeviceDiag = []string{"memtag_heap"}
		}),
	).RunTest(t)
	ctx := result.TestContext

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_binary_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_no_override", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_async", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("no_memtag_test_override_default_sync", variant), None)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_binary_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_no_override", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_async", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_disable", variant), Async)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_async_test_override_default_sync", variant), Async)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("set_memtag_set_sync_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_async", variant), Sync)
	// should sanitize: { diag: { memtag: true } } result in Sync instead of None here?
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_memtag_set_sync_test_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_disable", variant), None)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_binary_override_default_sync", variant), Sync)

	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_no_override", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_async", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_disable", variant), Sync)
	checkHasMemtagNote(t, ctx.ModuleForTests("unset_test_override_default_sync", variant), Sync)
}
