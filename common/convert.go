package common

import (
	"bytes"
	"encoding/binary"
)

// IntToBytes wacom
func IntToBytes(n int) []byte {
	tmp := int32(n)
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, tmp)
	return bytesBuffer.Bytes()
}

// BytesToInt wacom
func BytesToInt(b []byte) int {
	bytesBuffer := bytes.NewBuffer(b)
	var tmp int32
	binary.Read(bytesBuffer, binary.BigEndian, &tmp)
	return int(tmp)
}

// Uint32ToBytes wacom
func Uint32ToBytes(n uint32) []byte {
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, n)
	return bytesBuffer.Bytes()
}

// BytesToUint32 wacom
func BytesToUint32(b []byte) uint32 {
	bytesBuffer := bytes.NewBuffer(b)
	var n uint32
	binary.Read(bytesBuffer, binary.BigEndian, &n)
	return n
}

// Uint64ToBytes wacom
func Uint64ToBytes(n uint64) []byte {
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, n)
	return bytesBuffer.Bytes()
}

// BytesToUint64 wacom
func BytesToUint64(b []byte) uint64 {
	bytesBuffer := bytes.NewBuffer(b)
	var n uint64
	binary.Read(bytesBuffer, binary.BigEndian, &n)
	return n
}
