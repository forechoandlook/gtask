package version

var (
	Version = "0.1.1"
	Commit  = "dev"
)

func String() string {
	return Version + " (" + Commit + ")"
}
