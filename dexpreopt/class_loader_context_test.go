// Copyright 2020 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dexpreopt

// This file contains unit tests for class loader context structure.
// For class loader context tests involving .bp files, see TestUsesLibraries in java package.

import (
	"reflect"
	"strings"
	"testing"

	"android/soong/android"
)

func TestCLC(t *testing.T) {
	// Construct class loader context with the following structure:
	// .
	// ├── 29
	// │   ├── android.hidl.manager
	// │   └── android.hidl.base
	// │
	// └── any
	//     ├── a
	//     ├── b
	//     ├── c
	//     ├── d
	//     ├── a2
	//     ├── b2
	//     ├── c2
	//     ├── a1
	//     ├── b1
	//     ├── f
	//     ├── a3
	//     └── b3
	//
	ctx := testContext()

	lp := make(LibraryPaths)

	lp.AddLibraryPath(ctx, "a", buildPath(ctx, "a"), installPath(ctx, "a"))
	lp.AddLibraryPath(ctx, "b", buildPath(ctx, "b"), installPath(ctx, "b"))

	// "Maybe" variant in the good case: add as usual.
	c := "c"
	lp.MaybeAddLibraryPath(ctx, &c, buildPath(ctx, "c"), installPath(ctx, "c"))

	// "Maybe" variant in the bad case: don't add library with unknown name, keep going.
	lp.MaybeAddLibraryPath(ctx, nil, nil, nil)

	// Add some libraries with nested subcontexts.

	lp1 := make(LibraryPaths)
	lp1.AddLibraryPath(ctx, "a1", buildPath(ctx, "a1"), installPath(ctx, "a1"))
	lp1.AddLibraryPath(ctx, "b1", buildPath(ctx, "b1"), installPath(ctx, "b1"))

	lp2 := make(LibraryPaths)
	lp2.AddLibraryPath(ctx, "a2", buildPath(ctx, "a2"), installPath(ctx, "a2"))
	lp2.AddLibraryPath(ctx, "b2", buildPath(ctx, "b2"), installPath(ctx, "b2"))
	lp2.AddLibraryPath(ctx, "c2", buildPath(ctx, "c2"), installPath(ctx, "c2"))
	lp2.AddLibraryPaths(lp1)

	lp.AddLibraryPath(ctx, "d", buildPath(ctx, "d"), installPath(ctx, "d"))
	lp.AddLibraryPaths(lp2)

	lp3 := make(LibraryPaths)
	lp3.AddLibraryPath(ctx, "f", buildPath(ctx, "f"), installPath(ctx, "f"))
	lp3.AddLibraryPath(ctx, "a3", buildPath(ctx, "a3"), installPath(ctx, "a3"))
	lp3.AddLibraryPath(ctx, "b3", buildPath(ctx, "b3"), installPath(ctx, "b3"))
	lp.AddLibraryPaths(lp3)

	// Compatibility libraries with unknown install paths get default paths.
	lp.AddLibraryPath(ctx, AndroidHidlBase, buildPath(ctx, AndroidHidlBase), nil)
	lp.AddLibraryPath(ctx, AndroidHidlManager, buildPath(ctx, AndroidHidlManager), nil)
	lp.AddLibraryPath(ctx, AndroidTestMock, buildPath(ctx, AndroidTestMock), nil)

	module := testSystemModuleConfig(ctx, "test")
	module.LibraryPaths = lp

	m := make(classLoaderContextMap)
	valid := true

	ok, err := m.addLibs(ctx, AnySdkVersion, module, "a", "b", "c", "d", "a2", "b2", "c2", "a1", "b1", "f", "a3", "b3")
	valid = valid && ok && err == nil

	// Add compatibility libraries to conditional CLC for SDK level 29.
	ok, err = m.addLibs(ctx, 29, module, AndroidHidlManager, AndroidHidlBase)
	valid = valid && ok && err == nil

	// Add "android.test.mock" to conditional CLC, observe that is gets removed because it is only
	// needed as a compatibility library if "android.test.runner" is in CLC as well.
	ok, err = m.addLibs(ctx, 30, module, AndroidTestMock)
	valid = valid && ok && err == nil

	// When the same library is both in conditional and unconditional context, it should be removed
	// from conditional context.
	ok, err = m.addLibs(ctx, 42, module, "f")
	valid = valid && ok && err == nil

	fixConditionalClassLoaderContext(m)

	var haveStr string
	var havePaths android.Paths
	var haveUsesLibs []string
	if valid {
		haveStr, havePaths = computeClassLoaderContext(ctx, m)
		haveUsesLibs = m.usesLibs()
	}

	// Test that validation is successful (all paths are known).
	t.Run("validate", func(t *testing.T) {
		if !valid {
			t.Errorf("invalid class loader context")
		}
	})

	// Test that class loader context structure is correct.
	t.Run("string", func(t *testing.T) {
		wantStr := " --host-context-for-sdk 29 " +
			"PCL[out/" + AndroidHidlManager + ".jar]#" +
			"PCL[out/" + AndroidHidlBase + ".jar]" +
			" --target-context-for-sdk 29 " +
			"PCL[/system/framework/" + AndroidHidlManager + ".jar]#" +
			"PCL[/system/framework/" + AndroidHidlBase + ".jar]" +
			" --host-context-for-sdk any " +
			"PCL[out/a.jar]#PCL[out/b.jar]#PCL[out/c.jar]#PCL[out/d.jar]#" +
			"PCL[out/a2.jar]#PCL[out/b2.jar]#PCL[out/c2.jar]#" +
			"PCL[out/a1.jar]#PCL[out/b1.jar]#" +
			"PCL[out/f.jar]#PCL[out/a3.jar]#PCL[out/b3.jar]" +
			" --target-context-for-sdk any " +
			"PCL[/system/a.jar]#PCL[/system/b.jar]#PCL[/system/c.jar]#PCL[/system/d.jar]#" +
			"PCL[/system/a2.jar]#PCL[/system/b2.jar]#PCL[/system/c2.jar]#" +
			"PCL[/system/a1.jar]#PCL[/system/b1.jar]#" +
			"PCL[/system/f.jar]#PCL[/system/a3.jar]#PCL[/system/b3.jar]"
		if wantStr != haveStr {
			t.Errorf("\nwant class loader context: %s\nhave class loader context: %s", wantStr, haveStr)
		}
	})

	// Test that all expected build paths are gathered.
	t.Run("paths", func(t *testing.T) {
		wantPaths := []string{
			"out/android.hidl.manager-V1.0-java.jar", "out/android.hidl.base-V1.0-java.jar",
			"out/a.jar", "out/b.jar", "out/c.jar", "out/d.jar",
			"out/a2.jar", "out/b2.jar", "out/c2.jar",
			"out/a1.jar", "out/b1.jar",
			"out/f.jar", "out/a3.jar", "out/b3.jar",
		}
		if !reflect.DeepEqual(wantPaths, havePaths.Strings()) {
			t.Errorf("\nwant paths: %s\nhave paths: %s", wantPaths, havePaths)
		}
	})

	// Test for libraries that are added by the manifest_fixer.
	t.Run("uses libs", func(t *testing.T) {
		wantUsesLibs := []string{"a", "b", "c", "d", "a2", "b2", "c2", "a1", "b1", "f", "a3", "b3"}
		if !reflect.DeepEqual(wantUsesLibs, haveUsesLibs) {
			t.Errorf("\nwant uses libs: %s\nhave uses libs: %s", wantUsesLibs, haveUsesLibs)
		}
	})
}

// Test that an unexpected unknown build path causes immediate error.
func TestCLCUnknownBuildPath(t *testing.T) {
	ctx := testContext()
	lp := make(LibraryPaths)
	err := lp.addLibraryPath(ctx, "a", nil, nil, true)
	checkError(t, err, "unknown build path to <uses-library> 'a'")
}

// Test that an unexpected unknown install path causes immediate error.
func TestCLCUnknownInstallPath(t *testing.T) {
	ctx := testContext()
	lp := make(LibraryPaths)
	err := lp.addLibraryPath(ctx, "a", buildPath(ctx, "a"), nil, true)
	checkError(t, err, "unknown install path to <uses-library> 'a'")
}

func TestCLCMaybeAdd(t *testing.T) {
	ctx := testContext()

	lp := make(LibraryPaths)
	a := "a"
	lp.MaybeAddLibraryPath(ctx, &a, nil, nil)

	module := testSystemModuleConfig(ctx, "test")
	module.LibraryPaths = lp

	m := make(classLoaderContextMap)
	_, err := m.addLibs(ctx, AnySdkVersion, module, "a")
	checkError(t, err, "dexpreopt cannot find path for <uses-library> 'a'")
}

func checkError(t *testing.T, have error, want string) {
	if have == nil {
		t.Errorf("\nwant error: '%s'\nhave: none", want)
	} else if msg := have.Error(); !strings.HasPrefix(msg, want) {
		t.Errorf("\nwant error: '%s'\nhave error: '%s'\n", want, msg)
	}
}

func testContext() android.ModuleInstallPathContext {
	config := android.TestConfig("out", nil, "", nil)
	return android.ModuleInstallPathContextForTesting(config)
}

func buildPath(ctx android.PathContext, lib string) android.Path {
	return android.PathForOutput(ctx, lib+".jar")
}

func installPath(ctx android.ModuleInstallPathContext, lib string) android.InstallPath {
	return android.PathForModuleInstall(ctx, lib+".jar")
}