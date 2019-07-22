package common

var (
	DebugMode = false
)

func MinUint64(x, y uint64) uint64 {
	if x <= y {
		return x
	}
	return y
}

func MaxUint64(x, y uint64) uint64 {
	if x < y {
		return y
	}
	return x
}
