// Copyright 2016 Google Inc. All rights reserved.
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

package cc

import (
	"fmt"
	"strings"

	"github.com/google/blueprint"

	"android/soong"
	"android/soong/android"
	"android/soong/cc/config"
)

func init() {
	soong.RegisterModuleType("ndk_prebuilt_library", ndkPrebuiltLibraryFactory)
	soong.RegisterModuleType("ndk_prebuilt_object", ndkPrebuiltObjectFactory)
	soong.RegisterModuleType("ndk_prebuilt_static_stl", ndkPrebuiltStaticStlFactory)
	soong.RegisterModuleType("ndk_prebuilt_shared_stl", ndkPrebuiltSharedStlFactory)
}

// NDK prebuilt libraries.
//
// These differ from regular prebuilts in that they aren't stripped and usually aren't installed
// either (with the exception of the shared STLs, which are installed to the app's directory rather
// than to the system image).

func getNdkLibDir(ctx android.ModuleContext, toolchain config.Toolchain, version string) android.SourcePath {
	suffix := ""
	// Most 64-bit NDK prebuilts store libraries in "lib64", except for arm64 which is not a
	// multilib toolchain and stores the libraries in "lib".
	if toolchain.Is64Bit() && ctx.Arch().ArchType != android.Arm64 {
		suffix = "64"
	}
	return android.PathForSource(ctx, fmt.Sprintf("prebuilts/ndk/current/platforms/android-%s/arch-%s/usr/lib%s",
		version, toolchain.Name(), suffix))
}

func ndkPrebuiltModuleToPath(ctx android.ModuleContext, toolchain config.Toolchain,
	ext string, version string) android.Path {

	// NDK prebuilts are named like: ndk_NAME.EXT.SDK_VERSION.
	// We want to translate to just NAME.EXT
	name := strings.Split(strings.TrimPrefix(ctx.ModuleName(), "ndk_"), ".")[0]
	dir := getNdkLibDir(ctx, toolchain, version)
	return dir.Join(ctx, name+ext)
}

type ndkPrebuiltObjectLinker struct {
	objectLinker
}

func (*ndkPrebuiltObjectLinker) linkerDeps(ctx BaseModuleContext, deps Deps) Deps {
	// NDK objects can't have any dependencies
	return deps
}

func ndkPrebuiltObjectFactory() (blueprint.Module, []interface{}) {
	module := newBaseModule(android.DeviceSupported, android.MultilibBoth)
	module.linker = &ndkPrebuiltObjectLinker{}
	module.Properties.HideFromMake = true
	return module.Init()
}

func (c *ndkPrebuiltObjectLinker) link(ctx ModuleContext, flags Flags,
	deps PathDeps, objFiles android.Paths) android.Path {
	// A null build step, but it sets up the output path.
	if !strings.HasPrefix(ctx.ModuleName(), "ndk_crt") {
		ctx.ModuleErrorf("NDK prebuilts must have an ndk_crt prefixed name")
	}

	return ndkPrebuiltModuleToPath(ctx, flags.Toolchain, objectExtension, ctx.sdkVersion())
}

type ndkPrebuiltLibraryLinker struct {
	libraryLinker
}

var _ baseLinkerInterface = (*ndkPrebuiltLibraryLinker)(nil)
var _ exportedFlagsProducer = (*libraryLinker)(nil)

func (ndk *ndkPrebuiltLibraryLinker) linkerProps() []interface{} {
	return append(ndk.libraryLinker.linkerProps(), &ndk.Properties, &ndk.flagExporter.Properties)
}

func (*ndkPrebuiltLibraryLinker) linkerDeps(ctx BaseModuleContext, deps Deps) Deps {
	// NDK libraries can't have any dependencies
	return deps
}

func ndkPrebuiltLibraryFactory() (blueprint.Module, []interface{}) {
	module := newBaseModule(android.DeviceSupported, android.MultilibBoth)
	linker := &ndkPrebuiltLibraryLinker{}
	linker.dynamicProperties.BuildShared = true
	module.linker = linker
	module.Properties.HideFromMake = true
	return module.Init()
}

func (ndk *ndkPrebuiltLibraryLinker) link(ctx ModuleContext, flags Flags,
	deps PathDeps, objFiles android.Paths) android.Path {
	// A null build step, but it sets up the output path.
	ndk.exportIncludes(ctx, "-isystem")

	return ndkPrebuiltModuleToPath(ctx, flags.Toolchain, flags.Toolchain.ShlibSuffix(),
		ctx.sdkVersion())
}

// The NDK STLs are slightly different from the prebuilt system libraries:
//     * Are not specific to each platform version.
//     * The libraries are not in a predictable location for each STL.

type ndkPrebuiltStlLinker struct {
	ndkPrebuiltLibraryLinker
}

func ndkPrebuiltSharedStlFactory() (blueprint.Module, []interface{}) {
	module := newBaseModule(android.DeviceSupported, android.MultilibBoth)
	linker := &ndkPrebuiltStlLinker{}
	linker.dynamicProperties.BuildShared = true
	module.linker = linker
	module.Properties.HideFromMake = true
	return module.Init()
}

func ndkPrebuiltStaticStlFactory() (blueprint.Module, []interface{}) {
	module := newBaseModule(android.DeviceSupported, android.MultilibBoth)
	linker := &ndkPrebuiltStlLinker{}
	linker.dynamicProperties.BuildStatic = true
	module.linker = linker
	module.Properties.HideFromMake = true
	return module.Init()
}

func getNdkStlLibDir(ctx android.ModuleContext, toolchain config.Toolchain, stl string) android.SourcePath {
	gccVersion := toolchain.GccVersion()
	var libDir string
	switch stl {
	case "libstlport":
		libDir = "cxx-stl/stlport/libs"
	case "libc++":
		libDir = "cxx-stl/llvm-libc++/libs"
	case "libgnustl":
		libDir = fmt.Sprintf("cxx-stl/gnu-libstdc++/%s/libs", gccVersion)
	}

	if libDir != "" {
		ndkSrcRoot := "prebuilts/ndk/current/sources"
		return android.PathForSource(ctx, ndkSrcRoot).Join(ctx, libDir, ctx.Arch().Abi[0])
	}

	ctx.ModuleErrorf("Unknown NDK STL: %s", stl)
	return android.PathForSource(ctx, "")
}

func (ndk *ndkPrebuiltStlLinker) link(ctx ModuleContext, flags Flags,
	deps PathDeps, objFiles android.Paths) android.Path {
	// A null build step, but it sets up the output path.
	if !strings.HasPrefix(ctx.ModuleName(), "ndk_lib") {
		ctx.ModuleErrorf("NDK prebuilts must have an ndk_lib prefixed name")
	}

	ndk.exportIncludes(ctx, "-I")

	libName := strings.TrimPrefix(ctx.ModuleName(), "ndk_")
	libExt := flags.Toolchain.ShlibSuffix()
	if ndk.dynamicProperties.BuildStatic {
		libExt = staticLibraryExtension
	}

	stlName := strings.TrimSuffix(libName, "_shared")
	stlName = strings.TrimSuffix(stlName, "_static")
	libDir := getNdkStlLibDir(ctx, flags.Toolchain, stlName)
	return libDir.Join(ctx, libName+libExt)
}

func linkageMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(*Module); ok {
		if m.linker != nil {
			if linker, ok := m.linker.(baseLinkerInterface); ok {
				var modules []blueprint.Module
				if linker.buildStatic() && linker.buildShared() {
					modules = mctx.CreateLocalVariations("static", "shared")
					static := modules[0].(*Module)
					shared := modules[1].(*Module)

					static.linker.(baseLinkerInterface).setStatic(true)
					shared.linker.(baseLinkerInterface).setStatic(false)

					if staticCompiler, ok := static.compiler.(*libraryCompiler); ok {
						sharedCompiler := shared.compiler.(*libraryCompiler)
						if len(staticCompiler.Properties.Static.Cflags) == 0 &&
							len(sharedCompiler.Properties.Shared.Cflags) == 0 {
							// Optimize out compiling common .o files twice for static+shared libraries
							mctx.AddInterVariantDependency(reuseObjTag, shared, static)
							sharedCompiler.baseCompiler.Properties.Srcs = nil
							sharedCompiler.baseCompiler.Properties.Generated_sources = nil
						}
					}
				} else if linker.buildStatic() {
					modules = mctx.CreateLocalVariations("static")
					modules[0].(*Module).linker.(baseLinkerInterface).setStatic(true)
				} else if linker.buildShared() {
					modules = mctx.CreateLocalVariations("shared")
					modules[0].(*Module).linker.(baseLinkerInterface).setStatic(false)
				} else {
					panic(fmt.Errorf("library %q not static or shared", mctx.ModuleName()))
				}
			}
		}
	}
}
