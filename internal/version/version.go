package version

var (
	Version = "dev"
	Commit  = "dev"
)

func String() string {
	return Version + " (" + Commit + ")"
}
