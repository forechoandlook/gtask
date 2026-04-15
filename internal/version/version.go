package version

var (
	Version = "0.1.0"
	Commit  = "dev"
)

func String() string {
	return Version + " (" + Commit + ")"
}
