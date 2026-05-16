// Package relmapx reads relmap changelog exports and `relmap version
// --json` build states from an fs.FS, so tools can go:embed them.
// Unknown fields are ignored; only a schema other than 1 is refused.
package relmapx

// SchemaVersion is the export schema this package understands.
const SchemaVersion = 1

// File names relmap export writes into its output directory.
const (
	NDJSONFile   = "changelog.ndjson"
	IndexFile    = "changelog.index.json"
	MetadataFile = "changelog.metadata.json"
	MarkdownFile = "CHANGELOG.md"
)

// Header is the first record of changelog.ndjson.
type Header struct {
	Schema     int        `json:"schema"`
	Categories []string   `json:"categories"`
	Scope      Scope      `json:"scope"`
	State      BuildFlags `json:"state"`
}

// Scope records the flags the export ran with.
type Scope struct {
	Components     []string `json:"components"`
	DirectOnly     bool     `json:"direct_only"`
	IncludePending bool     `json:"include_pending"`
}

// BuildFlags is the state of the build an export was embedded in, as
// recorded by relmap (the `state` object of the header and of
// `relmap version --json`).
type BuildFlags struct {
	Unknown             bool     `json:"unknown"`
	Dirty               bool     `json:"dirty"`
	UndeclaredEvaluated bool     `json:"undeclared_evaluated"`
	Undeclared          []string `json:"undeclared"`
	ShortHash           string   `json:"short_hash,omitempty"`
}

// Change is one declared change record.
type Change struct {
	ID        string       `json:"id"`
	Date      string       `json:"date"`
	Category  string       `json:"category"`
	Impact    string       `json:"impact"`
	Summary   string       `json:"summary"`
	Detail    string       `json:"detail,omitempty"`
	Migration string       `json:"migration,omitempty"`
	Commits   []string     `json:"commits"`
	Affected  []Affected   `json:"affected"`
	Notes     []ChangeNote `json:"notes"`
	// Release is the "<line>@<version>" id of the release that includes
	// the change, empty while it is pending.
	Release string `json:"release"`
}

// Affected names a component a change reaches and the path it took.
type Affected struct {
	Component string   `json:"component"`
	Direct    bool     `json:"direct"`
	Via       []string `json:"via"`
}

// ChangeNote is a per-unit note attached to a change.
type ChangeNote struct {
	Unit string `json:"unit"`
	Text string `json:"text"`
}

// Release is one release record.
type Release struct {
	ID         string             `json:"id"`
	Line       string             `json:"line"`
	Version    string             `json:"version"`
	Channel    string             `json:"channel"`
	Date       string             `json:"date"`
	Stable     bool               `json:"stable"`
	Current    bool               `json:"current"`
	Summary    string             `json:"summary"`
	Paragraph  string             `json:"paragraph,omitempty"`
	Base       string             `json:"base,omitempty"`
	Promotes   string             `json:"promotes,omitempty"`
	SealedAt   string             `json:"sealed_at,omitempty"`
	Source     string             `json:"source,omitempty"`
	Commits    CommitSpan         `json:"commits"`
	Sections   []Section          `json:"sections"`
	Components []ReleaseComponent `json:"components"`
	Compat     []Compat           `json:"compat"`
	Deferrals  []Deferral         `json:"deferrals"`
}

// CommitSpan summarizes the commits a release covers.
type CommitSpan struct {
	Count  int    `json:"count"`
	Latest string `json:"latest"`
}

// Section groups a release's items under one change category.
type Section struct {
	Category string `json:"category"`
	Items    []Item `json:"items"`
}

// Item is one bullet; Covers names the change id it stands for.
type Item struct {
	Covers string `json:"covers,omitempty"`
	Text   string `json:"text"`
}

// ReleaseComponent is a component a release ships, with optional
// curated notes.
type ReleaseComponent struct {
	Component string          `json:"component"`
	Notes     *ComponentNotes `json:"notes,omitempty"`
}

// ComponentNotes are curated per-component release notes.
type ComponentNotes struct {
	Paragraph string `json:"paragraph,omitempty"`
	Items     []Item `json:"items"`
}

// Compat declares a compatible version range of another line.
type Compat struct {
	Line  string `json:"line"`
	Range string `json:"range"`
}

// Deferral explains changes held back from a release.
type Deferral struct {
	Changes []string `json:"changes"`
	Reason  string   `json:"reason"`
}

// Index is changelog.index.json: lookups into changelog.ndjson by
// record line number (the header is line 0).
type Index struct {
	Schema      int              `json:"schema"`
	NDJSON      IndexedFile      `json:"ndjson"`
	ByVersion   map[string]int   `json:"by_version"`
	ByComponent map[string][]int `json:"by_component"`
	ByCommit    map[string][]int `json:"by_commit"`
	ByDate      map[string][]int `json:"by_date"`
	// Ranges holds [byte offset, byte length] per ndjson line.
	Ranges [][2]int `json:"ranges"`
}

// IndexedFile identifies the ndjson an index was built over.
type IndexedFile struct {
	Bytes int    `json:"bytes"`
	Hash  string `json:"hash"`
	Lines int    `json:"lines"`
}

// Metadata is changelog.metadata.json.
type Metadata struct {
	Lines []LineMetadata `json:"lines"`
}

// LineMetadata summarizes one release line.
type LineMetadata struct {
	Line       string          `json:"line"`
	Current    *CurrentRelease `json:"current"`
	Versions   []VersionEntry  `json:"versions"`
	WindowSize int             `json:"window_size"`
}

// CurrentRelease is a line's newest non-ephemeral release.
type CurrentRelease struct {
	Channel string `json:"channel"`
	Date    string `json:"date"`
	Stable  bool   `json:"stable"`
	Version string `json:"version"`
}

// VersionEntry is one standard version in a line's window.
type VersionEntry struct {
	Version string     `json:"version"`
	Date    string     `json:"date"`
	Summary string     `json:"summary"`
	Commits CommitSpan `json:"commits"`
}
