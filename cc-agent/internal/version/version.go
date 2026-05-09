package version

import "strings"

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

func String() string {
	return Format(Version, Commit, Date)
}

func Format(version, commit, date string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "dev"
	}

	metadata := make([]string, 0, 2)
	if commit = strings.TrimSpace(commit); commit != "" {
		if len(commit) > 7 {
			commit = commit[:7]
		}
		metadata = append(metadata, commit)
	}
	if date = strings.TrimSpace(date); date != "" {
		metadata = append(metadata, date)
	}

	out := "cc-agent " + version
	if len(metadata) > 0 {
		out += " (" + strings.Join(metadata, ", ") + ")"
	}
	return out
}
