package project

var (
	version = "dirty"
	commit  = "NA"
)

func Version() string {
	return version
}

func Commit() string {
	return commit
}
