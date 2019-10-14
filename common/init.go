package common

func InitTestnet() {
	UseTestnetRule = true

	VOTE1_FREEZE_TX_START = 640100
	VOTE1_FREEZE_TX_END = 646700
	Vote1RefundAddress = HexToAddress("0xf97a9980808a2cae0d09ff693f02a4f80abb22c4")
	Vote1DrainList = []Address{
		HexToAddress("0x07f35aba9555a532c0edc2bd6350c891b6f2c8d0"),
		HexToAddress("0x3dfaef310a1044fd7d96750b42b44cf3775c00bf"),
		HexToAddress("0x32095bb7f699a139036d68defd4f12b682795fb6"),
	}
}

func InitDevnet() {
	DebugMode = true
	UseDevnetRule = true

	//VOTE1_FREEZE_TX_START = 50
	//VOTE1_FREEZE_TX_END = 100
	//Vote1RefundAddress = HexToAddress("0xf97a9980808a2cae0d09ff693f02a4f80abb22c4")
	//Vote1DrainList = []Address{
	//	HexToAddress("0x07f35aba9555a532c0edc2bd6350c891b6f2c8d0"),
	//	HexToAddress("0x3dfaef310a1044fd7d96750b42b44cf3775c00bf"),
	//	HexToAddress("0x32095bb7f699a139036d68defd4f12b682795fb6"),
	//}
}
