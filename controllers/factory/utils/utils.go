package utils

import "hash/crc32"

func GetHash(input []byte) uint32 {
	crc32q := crc32.MakeTable(crc32.IEEE)
	return crc32.Checksum(input, crc32q)
}
