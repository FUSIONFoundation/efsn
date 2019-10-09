package common

func InitTestnet() {
	UseTestnetRule = true
}

func InitDevnet() {
	DebugMode = true
	UseDevnetRule = true
}
