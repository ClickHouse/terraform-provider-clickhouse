package tableBuilder

type Column struct {
	Name     string
	Type     string
	Nullable bool
	Default  string
	Codec    string
}
