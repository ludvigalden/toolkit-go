package fsenum

import (
	"path"
	"slices"
)

// Set is a group of names a walk can skip, matched against base names:
// Dirs exactly, DirGlobs and Files with path.Match. Each category
// constructor returns a fresh Set, so callers may extend it freely.
type Set struct {
	Dirs     []string
	DirGlobs []string
	Files    []string
}

// Union returns s with every entry of more appended, duplicates dropped.
func (s Set) Union(more ...Set) Set {
	out := Set{Dirs: slices.Clone(s.Dirs), DirGlobs: slices.Clone(s.DirGlobs), Files: slices.Clone(s.Files)}
	for _, m := range more {
		out.Dirs = appendNew(out.Dirs, m.Dirs)
		out.DirGlobs = appendNew(out.DirGlobs, m.DirGlobs)
		out.Files = appendNew(out.Files, m.Files)
	}
	return out
}

// Skip returns an Options.Skip func for s.
func (s Set) Skip() func(rel string, isDir bool) bool {
	dirs := make(map[string]struct{}, len(s.Dirs))
	for _, d := range s.Dirs {
		dirs[d] = struct{}{}
	}
	dirGlobs, files := slices.Clone(s.DirGlobs), slices.Clone(s.Files)
	return func(rel string, isDir bool) bool {
		base := path.Base(rel)
		if !isDir {
			return matchAny(files, base)
		}
		if _, ok := dirs[base]; ok {
			return true
		}
		return matchAny(dirGlobs, base)
	}
}

// Default is the convenience set most repository walkers want: every
// category below except GeneratedCode, Images and AgentConfig, which
// hold files that are often worth reading. Lockfiles and .env files are
// never skipped by any category.
func Default() Set {
	return VCS().Union(Dependencies(), BuildOutputs(), Caches(), Coverage(),
		Editors(), OSFiles(), Backups(), Binaries(), Compressed(), Databases(), Legacy())
}

// VCS holds version-control metadata directories.
func VCS() Set {
	return Set{Dirs: []string{".git", ".hg", ".svn", ".bzr", ".pijul", "_darcs"}}
}

// Dependencies holds installed third-party code and virtual environments.
func Dependencies() Set {
	return Set{Dirs: []string{
		"node_modules", "bower_components", ".bin", ".npm", ".pnpm-store", ".yarn",
		"vendor", "third_party",
		".venv", "venv", ".conda", ".mamba", ".eggs",
		".gradle", ".mvn", ".deno",
	}}
}

// BuildOutputs holds tool-specific build output. The generic names
// build, bin and target are left out: they are as often source.
func BuildOutputs() Set {
	return Set{
		Dirs: []string{
			"dist", "out", ".output", ".build", ".artifacts",
			".next", ".nuxt", ".svelte-kit", ".astro", ".wxt", ".angular",
			".docusaurus", ".react-router", ".vite", ".turbo", ".nx",
			".parcel-cache", ".rollup.cache", ".fusebox",
			".wrangler", ".mf", ".miniflare", ".vercel", ".netlify",
		},
		Files: []string{"*.tsbuildinfo", "*.min.js", "*.min.css", "*.min.js.map", "*.min.css.map"},
	}
}

// Caches holds caches and scratch space.
func Caches() Set {
	return Set{
		Dirs: []string{
			".cache", ".tmp", ".temp", ".temporary", "tmp", ".trash",
			"__pycache__", ".pytest_cache", ".mypy_cache", ".ruff_cache", ".tox", ".nox",
			".eslintcache",
		},
		DirGlobs: []string{".tmp-*", ".tmp_*", "tmp-*", "tmp_*", "_*_temp"},
		Files:    []string{".eslintcache", "*.tmp", "*.pyc", "*.pyo"},
	}
}

// Coverage holds coverage and test-report output.
func Coverage() Set {
	return Set{
		Dirs:  []string{"coverage", ".coverage", "htmlcov", ".nyc_output", "test-results", "playwright-report"},
		Files: []string{".coverage", "*.coverprofile", "coverage.out"},
	}
}

// Editors holds editor and IDE state.
func Editors() Set {
	return Set{
		Dirs:  []string{".idea", ".vscode", ".vs", ".zed", ".fleet"},
		Files: []string{"*.swp", "*.swo", "*~", "*.sublime-workspace"},
	}
}

// OSFiles holds operating-system metadata files.
func OSFiles() Set {
	return Set{Files: []string{".DS_Store", "Thumbs.db", "desktop.ini", "._*"}}
}

// Backups holds backup and merge-leftover files.
func Backups() Set {
	return Set{Files: []string{"*.bak", "*.bak[0-9]", "*.orig", "*.rej"}}
}

// Binaries holds compiled objects and executables.
func Binaries() Set {
	return Set{Files: []string{"*.exe", "*.dll", "*.so", "*.dylib", "*.a", "*.o", "*.obj", "*.lib", "*.wasm", "*.class", "*.jar"}}
}

// Compressed holds archives and compressed files.
func Compressed() Set {
	return Set{Files: []string{"*.zip", "*.gz", "*.tgz", "*.tar", "*.bz2", "*.xz", "*.zst", "*.7z", "*.rar"}}
}

// Databases holds embedded database files.
func Databases() Set {
	return Set{Files: []string{"*.db", "*.sqlite", "*.sqlite3", "*.db-shm", "*.db-wal"}}
}

// Legacy holds directories conventionally used to park retired code.
func Legacy() Set {
	return Set{
		Dirs:     []string{".legacy", "_legacy", ".archive", "_archive"},
		DirGlobs: []string{"_legacy-*", "_legacy_*", "_archive-*", "_archive_*"},
	}
}

// GeneratedCode holds generated source files (not in Default).
func GeneratedCode() Set {
	return Set{Files: []string{
		"*.pb.go", "*.pb.gw.go", "*.pb.grpc.go", "*_gen.go", "*.generated.go",
		"*.gen.ts", "*.gen.js", "*.generated.ts", "*_pb2.py", "*_pb2_grpc.py",
	}}
}

// Images holds raster and vector images (not in Default).
func Images() Set {
	return Set{Files: []string{"*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp", "*.ico", "*.svg", "*.bmp", "*.tiff", "*.avif"}}
}

// AgentConfig holds generated coding-assistant configuration (not in
// Default).
func AgentConfig() Set {
	return Set{
		Dirs:     []string{".claude", ".codex", ".gemini", ".cursor", ".windsurf", ".agents"},
		DirGlobs: []string{".claude-*", ".agents-*"},
	}
}

// appendNew appends the entries of add missing from dst.
func appendNew(dst, add []string) []string {
	for _, a := range add {
		if !slices.Contains(dst, a) {
			dst = append(dst, a)
		}
	}
	return dst
}

// matchAny reports whether name matches any glob.
func matchAny(globs []string, name string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, name); ok {
			return true
		}
	}
	return false
}
