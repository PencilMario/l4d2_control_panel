package disk

type Checker struct{}

func (Checker) Available(path string) (uint64, error) { return Available(path) }
