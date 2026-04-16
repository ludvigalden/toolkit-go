// Package fsenum enumerates files without an index: a metadata-only walk,
// changed-since queries, and marker-file discovery. Queries use fd when it
// is installed and fall back to fs.WalkDir; both honor .gitignore/.ignore.
package fsenum
