package inventory

// Script contains fields that are relevant to all of the executable scripts mango will be working with.
// - ID: string identifying the script (the name of the script)
// - Path: Absolute path to the script
type Script struct {
	ID   string
	Path string
}
